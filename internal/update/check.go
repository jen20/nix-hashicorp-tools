package update

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"

	"github.com/jen20/nix-hashicorp-tools/internal/config"
	"github.com/jen20/nix-hashicorp-tools/internal/manifest"
	"github.com/jen20/nix-hashicorp-tools/internal/semver"
)

// Check validates the generated manifests without touching the network. It
// covers the invariants the Nix side relies on but cannot itself express:
// that the index agrees with the manifests, that version order is genuinely
// ascending, and that every pointer resolves.
func Check(cfg *config.Config, dataDir string) (problems []string, checked int, err error) {
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	platforms, err := readJSON[manifest.Platforms](filepath.Join(dataDir, manifest.PlatformsFile))
	if err != nil {
		return nil, 0, err
	}

	for _, platform := range cfg.Platforms {
		target, ok := platforms[platform.System]
		switch {
		case !ok:
			report("platforms.json: %s is configured but absent", platform.System)
		case target.OS != platform.OS || target.Arch != platform.Arch:
			report("platforms.json: %s is %s/%s but configured as %s/%s",
				platform.System, target.OS, target.Arch, platform.OS, platform.Arch)
		}
	}
	for system := range platforms {
		if _, ok := cfg.System(platforms[system].OS, platforms[system].Arch); !ok {
			report("platforms.json: %s is not configured", system)
		}
	}

	index, err := readJSON[manifest.Index](filepath.Join(dataDir, manifest.IndexFile))
	if err != nil {
		return nil, 0, err
	}

	for _, product := range cfg.Products {
		path := manifest.ProductPath(dataDir, product.Name)

		if _, statErr := os.Stat(path); statErr != nil {
			report("%s: no manifest at %s", product.Name, path)
			continue
		}

		m, readErr := manifest.Read(path)
		if readErr != nil {
			return nil, checked, readErr
		}

		checked += len(m.Versions)
		problems = append(problems, checkManifest(product, m, platforms)...)

		entry, ok := index[product.Name]
		if !ok {
			report("index.json: %s is missing", product.Name)
			continue
		}
		if !sameMap(entry.Latest, m.Latest) {
			report("index.json: %s latest pointers disagree with its manifest", product.Name)
		}
		if !sameMap(entry.LatestPrerelease, m.LatestPrerelease) {
			report("index.json: %s prerelease pointers disagree with its manifest", product.Name)
		}
	}

	for name := range index {
		if _, ok := cfg.Product(name); !ok {
			report("index.json: %s is not configured", name)
		}
	}

	slices.Sort(problems)

	return problems, checked, nil
}

func checkManifest(product config.Product, m *manifest.Manifest, platforms manifest.Platforms) []string {
	var problems []string

	report := func(format string, args ...any) {
		problems = append(problems, product.Name+": "+fmt.Sprintf(format, args...))
	}

	if m.Product != product.Name {
		report("manifest names the product %q", m.Product)
	}
	if m.MainProgram != product.Program() {
		report("mainProgram is %q but %q is configured", m.MainProgram, product.Program())
	}

	if len(m.Order) != len(m.Versions) {
		report("order lists %d versions but there are %d", len(m.Order), len(m.Versions))
	}
	for i, version := range m.Order {
		if _, ok := m.Versions[version]; !ok {
			report("order lists %s, which has no release", version)
		}
		if i > 0 && semver.Compare(m.Order[i-1], version) > 0 {
			report("order is not ascending at %s", version)
		}
	}

	for i, version := range m.Unavailable {
		if _, ok := m.Versions[version]; ok {
			report("%s is recorded both as a release and as unavailable", version)
		}
		if i > 0 && semver.Compare(m.Unavailable[i-1], version) > 0 {
			report("unavailable is not ascending at %s", version)
		}
	}

	for version, release := range m.Versions {
		if release.License == "" {
			report("%s has no licence", version)
		}
		if len(release.Platforms) == 0 {
			report("%s has no platforms", version)
		}
		for system, sum := range release.Platforms {
			if _, ok := platforms[system]; !ok {
				report("%s targets unknown system %s", version, system)
			}
			if !isSHA256(sum) {
				report("%s has a malformed hash for %s", version, system)
			}
		}
	}

	checkPointers := func(kind string, pointers map[string]string, wantPrerelease bool) {
		for system, version := range pointers {
			release, ok := m.Versions[version]
			if !ok {
				report("%s pointer for %s names %s, which has no release", kind, system, version)
				continue
			}
			if _, ok := release.Platforms[system]; !ok {
				report("%s pointer for %s names %s, which has no build for it", kind, system, version)
			}
			if release.Prerelease != wantPrerelease {
				report("%s pointer for %s names %s, whose prerelease flag disagrees", kind, system, version)
			}
		}
	}

	checkPointers("latest", m.Latest, false)
	checkPointers("latestPrerelease", m.LatestPrerelease, true)

	if len(m.Latest) == 0 {
		report("no latest pointers")
	}

	return problems
}

// List writes a summary of the tracked products and their newest releases.
func List(cfg *config.Config, dataDir string, out io.Writer) error {
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	// tabwriter buffers, so any write error surfaces from Flush.
	_, _ = fmt.Fprintln(writer, "PRODUCT\tRELEASES\tLATEST\tPRERELEASE\tSYSTEMS")

	for _, product := range cfg.Products {
		m, err := manifest.Read(manifest.ProductPath(dataDir, product.Name))
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%s\t%d\n",
			product.Name,
			len(m.Versions),
			newest(m.Latest),
			newest(m.LatestPrerelease),
			len(m.Latest),
		)
	}

	return writer.Flush()
}

// newest returns the highest version among a set of per-system pointers.
func newest(pointers map[string]string) string {
	best := "-"
	for _, version := range pointers {
		if best == "-" || semver.Less(best, version) {
			best = version
		}
	}
	return best
}

func sameMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func readJSON[T any](path string) (T, error) {
	var value T

	contents, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}

	if err := json.Unmarshal(contents, &value); err != nil {
		return value, fmt.Errorf("parsing %s: %w", path, err)
	}

	return value, nil
}
