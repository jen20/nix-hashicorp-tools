// Package config describes which products the overlay tracks and how release
// artefacts map onto Nix systems.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config is the updater's input, read from products.json.
//
// JSON rather than TOML so that the module keeps the releases client as its
// only dependency: the standard library has no TOML decoder.
type Config struct {
	// Platforms maps release artefacts onto Nix systems. Artefacts built for
	// a target with no entry here are ignored.
	Platforms []Platform `json:"platforms"`

	// Products lists the products to track, in the order they are processed.
	Products []Product `json:"products"`
}

// Platform associates a Nix system double with the Go OS and architecture
// names that HashiCorp uses in release archive filenames.
type Platform struct {
	System string `json:"system"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
}

// Product is a single tracked product.
type Product struct {
	// Name is the product name as used by the releases API.
	Name string `json:"name"`

	// Description is used for meta.description. The releases API does not
	// carry one.
	Description string `json:"description"`

	// MainProgram is the binary that meta.mainProgram should point at. It
	// defaults to Name.
	MainProgram string `json:"mainProgram,omitempty"`

	// License is the nixpkgs lib.licenses attribute name to record for a
	// release whose licence cannot be resolved from source, and for every
	// release when ResolveLicense is false.
	License string `json:"license"`

	// ResolveLicense requests that each release's licence be read from the
	// LICENSE file at its tag in the source repository.
	//
	// HashiCorp relicensed maintenance branches under BUSL at different
	// points per branch, so licence is not monotonic in version order:
	// consul 1.15.10 is BUSL while the later 1.16.1 is MPL. A version
	// cut-over table therefore cannot express reality, and the tag is the
	// only cheap authoritative source. Results are cached in the manifest,
	// so this costs nothing once a release has been seen.
	ResolveLicense bool `json:"resolveLicense"`

	// TagTemplate renders a version into a source repository tag. "{version}"
	// is substituted. Defaults to "v{version}".
	TagTemplate string `json:"tagTemplate,omitempty"`
}

// Program returns the binary name to record as meta.mainProgram.
func (p Product) Program() string {
	if p.MainProgram != "" {
		return p.MainProgram
	}
	return p.Name
}

// Tag renders the source repository tag for a version.
func (p Product) Tag(version string) string {
	template := p.TagTemplate
	if template == "" {
		template = "v{version}"
	}
	return strings.ReplaceAll(template, "{version}", version)
}

// Load reads and validates a configuration file.
func Load(path string) (*Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Platforms) == 0 {
		return fmt.Errorf("no platforms configured")
	}
	if len(c.Products) == 0 {
		return fmt.Errorf("no products configured")
	}

	systems := make(map[string]struct{}, len(c.Platforms))
	targets := make(map[string]struct{}, len(c.Platforms))
	for _, platform := range c.Platforms {
		if platform.System == "" || platform.OS == "" || platform.Arch == "" {
			return fmt.Errorf("platform %+v has an empty field", platform)
		}
		if _, seen := systems[platform.System]; seen {
			return fmt.Errorf("duplicate platform system %q", platform.System)
		}
		systems[platform.System] = struct{}{}

		target := platform.OS + "/" + platform.Arch
		if _, seen := targets[target]; seen {
			return fmt.Errorf("duplicate platform target %q", target)
		}
		targets[target] = struct{}{}
	}

	names := make(map[string]struct{}, len(c.Products))
	for _, product := range c.Products {
		switch {
		case product.Name == "":
			return fmt.Errorf("product with no name")
		case product.License == "":
			return fmt.Errorf("product %q has no license", product.Name)
		case product.Description == "":
			return fmt.Errorf("product %q has no description", product.Name)
		}
		if _, seen := names[product.Name]; seen {
			return fmt.Errorf("duplicate product %q", product.Name)
		}
		names[product.Name] = struct{}{}
	}

	return nil
}

// System returns the Nix system double for a release artefact's OS and
// architecture, and reports whether the target is one the overlay covers.
func (c *Config) System(os, arch string) (string, bool) {
	for _, platform := range c.Platforms {
		if platform.OS == os && platform.Arch == arch {
			return platform.System, true
		}
	}
	return "", false
}

// Product returns the configuration for a named product.
func (c *Config) Product(name string) (Product, bool) {
	for _, product := range c.Products {
		if product.Name == name {
			return product, true
		}
	}
	return Product{}, false
}

// Names returns the configured product names in order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Products))
	for _, product := range c.Products {
		names = append(names, product.Name)
	}
	return names
}
