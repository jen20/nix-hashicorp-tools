// Package manifest defines the generated data files that the Nix overlay
// reads, and the routines for reading and writing them reproducibly.
package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jen20/nix-hashicorp-tools/internal/semver"
)

const (
	// ProductsDir is the directory beneath the data root holding one manifest
	// per product.
	ProductsDir = "products"

	// IndexFile names the tree index, relative to the data root.
	IndexFile = "index.json"

	// PlatformsFile names the platform map, relative to the data root.
	PlatformsFile = "platforms.json"
)

// Manifest records every tracked release of a single product.
type Manifest struct {
	Product          string `json:"product"`
	Description      string `json:"description,omitempty"`
	Homepage         string `json:"homepage,omitempty"`
	MainProgram      string `json:"mainProgram"`
	SourceRepository string `json:"sourceRepository,omitempty"`

	// Latest and LatestPrerelease map a Nix system to the newest release
	// carrying a build for it. The newest release of a product does not
	// necessarily build for every system it once did, so these cannot be
	// single values.
	Latest           map[string]string `json:"latest"`
	LatestPrerelease map[string]string `json:"latestPrerelease,omitempty"`

	// Order lists every version in ascending precedence order. JSON objects
	// have no ordering, and sorting ~450 versions in Nix is needlessly
	// expensive, so the updater records the order it has already computed.
	Order []string `json:"order"`

	// Unavailable lists releases that were examined and found to have no
	// usable build for any tracked platform, either because they are built
	// only for untracked targets or because their artefacts deviate from the
	// canonical layout.
	//
	// Recording them is what keeps a routine run incremental: without it,
	// every one of them is re-fetched on every run, since none ever appears
	// in Versions. Deleting an entry from here makes the updater reconsider
	// it, which is the way to pick up a release HashiCorp has since fixed.
	Unavailable []string `json:"unavailable,omitempty"`

	Versions map[string]Release `json:"versions"`
}

// Release is a single release of a product.
type Release struct {
	Prerelease bool `json:"prerelease"`

	// License is a nixpkgs lib.licenses attribute name.
	License string `json:"license"`

	// Platforms maps a Nix system to the SHA-256 of that system's release
	// archive. HashiCorp publishes hashes of the archives themselves, which
	// is exactly what fetchurl consumes.
	Platforms map[string]string `json:"platforms"`
}

// Index lets Nix enumerate the tree, and resolve the per-system latest
// release of each product, without parsing every product manifest.
type Index map[string]IndexEntry

// IndexEntry is one product's contribution to the index.
type IndexEntry struct {
	Latest           map[string]string `json:"latest"`
	LatestPrerelease map[string]string `json:"latestPrerelease,omitempty"`
}

// Platforms maps a Nix system to the Go OS and architecture used to build the
// download URL for an archive.
type Platforms map[string]Target

// Target is a Go build target.
type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// ProductPath returns the path of a product's manifest beneath a data root.
func ProductPath(dataDir, product string) string {
	return filepath.Join(dataDir, ProductsDir, product+".json")
}

// Read loads a product manifest. A manifest that does not yet exist reads as
// an empty one, so that a first run and an incremental run take the same path.
func Read(path string) (*Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Versions: map[string]Release{}}, nil
		}
		return nil, err
	}

	var m Manifest
	if err := json.Unmarshal(contents, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m.Versions == nil {
		m.Versions = map[string]Release{}
	}

	return &m, nil
}

// Finalise recomputes the derived fields of a manifest: the ascending version
// order, and the per-system newest stable and prerelease pointers.
func (m *Manifest) Finalise() {
	m.Order = make([]string, 0, len(m.Versions))
	for version := range m.Versions {
		m.Order = append(m.Order, version)
	}
	sort.Slice(m.Order, func(i, j int) bool {
		if c := semver.Compare(m.Order[i], m.Order[j]); c != 0 {
			return c < 0
		}
		// Distinct strings of equal precedence still need a total order.
		return m.Order[i] < m.Order[j]
	})

	m.Latest = map[string]string{}
	m.LatestPrerelease = map[string]string{}

	// Walking in ascending order means the last write for each system wins.
	for _, version := range m.Order {
		release := m.Versions[version]

		target := m.Latest
		if release.Prerelease {
			target = m.LatestPrerelease
		}

		for system := range release.Platforms {
			target[system] = version
		}
	}

	if len(m.LatestPrerelease) == 0 {
		m.LatestPrerelease = nil
	}
}

// Entry returns the manifest's contribution to the index.
func (m *Manifest) Entry() IndexEntry {
	return IndexEntry{
		Latest:           m.Latest,
		LatestPrerelease: m.LatestPrerelease,
	}
}

// Marshal renders a value as the repository's canonical JSON: two-space
// indentation, no HTML escaping, and a trailing newline. Go sorts map keys
// when marshalling, so output is stable across runs and diffs stay minimal.
func Marshal(value any) ([]byte, error) {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Write renders a value to path, creating parent directories as needed. It
// reports whether the file's contents changed.
func Write(path string, value any) (bool, error) {
	encoded, err := Marshal(value)
	if err != nil {
		return false, err
	}

	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, encoded) {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}

	// Write via a temporary file so that an interrupted run cannot leave a
	// truncated manifest behind for Nix to choke on.
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return false, err
	}
	defer func() {
		_ = os.Remove(temp.Name())
	}()

	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return false, err
	}

	return true, os.Rename(temp.Name(), path)
}
