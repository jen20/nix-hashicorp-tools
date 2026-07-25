package update

import (
	"bytes"
	"fmt"
	"strings"
)

// licenceMarkers maps a distinctive phrase from the beginning of a licence
// text to the nixpkgs lib.licenses attribute name for it. BUSL is recognised
// by the MariaDB copyright line that opens every copy of it.
var licenceMarkers = []struct {
	marker  string
	nixName string
}{
	{"MariaDB Corporation Ab", "bsl11"},
	{"Business Source License", "bsl11"},
	{"Mozilla Public License", "mpl20"},
	{"Apache License", "asl20"},
	{"MIT License", "mit"},
}

// DetectLicense identifies a licence text and returns its nixpkgs
// lib.licenses attribute name.
func DetectLicense(contents []byte) (string, bool) {
	// Every marker appears in the preamble; reading further only risks
	// matching a licence quoted inside another one.
	head := contents
	if len(head) > 4096 {
		head = head[:4096]
	}

	for _, candidate := range licenceMarkers {
		if bytes.Contains(head, []byte(candidate.marker)) {
			return candidate.nixName, true
		}
	}

	return "", false
}

// rawURL rewrites a GitHub repository URL into a raw content URL for a file
// at a tag.
func rawURL(repository, tag, path string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(repository, "/"), ".git")

	prefix, found := strings.CutPrefix(trimmed, "https://github.com/")
	if !found {
		return "", fmt.Errorf("not a GitHub repository: %q", repository)
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", prefix, tag, path), nil
}
