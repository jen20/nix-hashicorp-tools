package update

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// ArchiveName returns the release archive filename that HashiCorp publishes
// for a build target.
func ArchiveName(product, version, os, arch string) string {
	return fmt.Sprintf("%s_%s_%s_%s.zip", product, version, os, arch)
}

// ArchiveURL returns the canonical download URL for a build target.
//
// The overlay derives this URL in Nix rather than storing it, so the updater
// asserts that the API agrees before recording a release; see
// Updater.release.
func ArchiveURL(product, version, os, arch string) string {
	return fmt.Sprintf(
		"https://releases.hashicorp.com/%s/%s/%s",
		product, version, ArchiveName(product, version, os, arch),
	)
}

// ParseSHASums reads a HashiCorp SHA256SUMS file into a map of archive
// filename to hex-encoded SHA-256.
func ParseSHASums(contents []byte) (map[string]string, error) {
	sums := map[string]string{}

	scanner := bufio.NewScanner(bytes.NewReader(contents))
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}

		// Lines are "<64 hex characters><whitespace><filename>". The
		// separator is two spaces in practice, but coreutils also uses
		// " *" for binary mode, so split on any whitespace run.
		sum, name, found := strings.Cut(text, " ")
		if !found {
			return nil, fmt.Errorf("line %d: no separator", line)
		}
		name = strings.TrimLeft(name, " *\t")

		if !isSHA256(sum) {
			return nil, fmt.Errorf("line %d: %q is not a SHA-256", line, sum)
		}
		if name == "" {
			return nil, fmt.Errorf("line %d: no filename", line)
		}

		sums[name] = sum
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, fmt.Errorf("no checksums found")
	}

	return sums, nil
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
