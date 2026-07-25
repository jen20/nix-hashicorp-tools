package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SignatureResult is the outcome of checking a detached signature.
type SignatureResult int

const (
	// SignatureGood means the signature was made by a trusted key.
	SignatureGood SignatureResult = iota

	// SignatureUnknownKey means the signature was made by a key that is not
	// in the keyring. HashiCorp rotated its signing key in April 2021 after
	// the Codecov incident and revoked the previous one, so releases made
	// before then cannot be verified with any published key.
	SignatureUnknownKey
)

// Verifier checks detached OpenPGP signatures by shelling out to gpg.
//
// The standard library has no OpenPGP support and golang.org/x/crypto/openpgp
// is deprecated and unmaintained, so shelling out is what keeps this module's
// dependency list down to the releases client.
type Verifier struct {
	home string
}

// NewVerifier imports every .asc file in keysDir into a throwaway GnuPG home
// directory. Call Close to remove it.
func NewVerifier(ctx context.Context, keysDir string) (*Verifier, error) {
	keys, err := filepath.Glob(filepath.Join(keysDir, "*.asc"))
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no keys in %s", keysDir)
	}

	home, err := os.MkdirTemp("", "hcnix-gnupg-*")
	if err != nil {
		return nil, err
	}
	// gpg refuses to use a home directory others can read.
	if err := os.Chmod(home, 0o700); err != nil {
		_ = os.RemoveAll(home)
		return nil, err
	}

	v := &Verifier{home: home}

	args := append([]string{"--batch", "--quiet", "--import"}, keys...)
	if _, err := v.gpg(ctx, args...); err != nil {
		_ = v.Close()
		return nil, fmt.Errorf("importing keys: %w", err)
	}

	return v, nil
}

// Close removes the temporary GnuPG home directory.
func (v *Verifier) Close() error {
	if v == nil || v.home == "" {
		return nil
	}
	return os.RemoveAll(v.home)
}

// Verify checks a detached signature over data.
func (v *Verifier) Verify(ctx context.Context, signature, data []byte) (SignatureResult, error) {
	dir, err := os.MkdirTemp(v.home, "verify-*")
	if err != nil {
		return SignatureUnknownKey, err
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	sigPath := filepath.Join(dir, "sums.sig")
	dataPath := filepath.Join(dir, "sums")
	if err := os.WriteFile(sigPath, signature, 0o600); err != nil {
		return SignatureUnknownKey, err
	}
	if err := os.WriteFile(dataPath, data, 0o600); err != nil {
		return SignatureUnknownKey, err
	}

	// --status-fd gives machine-readable results; gpg's exit code alone does
	// not distinguish "forged" from "key not held", and only the former
	// should ever be fatal.
	status, err := v.gpg(ctx, "--batch", "--status-fd", "1", "--verify", sigPath, dataPath)

	switch {
	case bytes.Contains(status, []byte("[GNUPG:] GOODSIG")):
		return SignatureGood, nil
	case bytes.Contains(status, []byte("[GNUPG:] NO_PUBKEY")):
		return SignatureUnknownKey, nil
	case bytes.Contains(status, []byte("[GNUPG:] BADSIG")):
		return SignatureUnknownKey, fmt.Errorf("signature is not valid for this content")
	case bytes.Contains(status, []byte("[GNUPG:] REVKEYSIG")):
		return SignatureUnknownKey, fmt.Errorf("signature was made by a revoked key")
	case bytes.Contains(status, []byte("[GNUPG:] EXPKEYSIG")):
		return SignatureUnknownKey, fmt.Errorf("signature was made by an expired key")
	case err != nil:
		return SignatureUnknownKey, err
	default:
		return SignatureUnknownKey, fmt.Errorf("gpg returned no verdict")
	}
}

// gpg runs gpg against the throwaway home directory and returns its standard
// output. A non-zero exit is reported as an error carrying gpg's diagnostics,
// but callers checking a signature should read the verdict out of the returned
// status output rather than relying on the exit code.
func (v *Verifier) gpg(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gpg", append([]string{"--homedir", v.home}, args...)...)

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	// Ensure gpg cannot pick up the invoking user's configuration.
	cmd.Env = append(os.Environ(), "GNUPGHOME="+v.home)

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(errOut.String())
		if message == "" {
			message = err.Error()
		}
		return out.Bytes(), fmt.Errorf("gpg: %s", message)
	}

	return out.Bytes(), nil
}
