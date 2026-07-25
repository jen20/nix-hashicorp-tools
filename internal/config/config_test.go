package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "products.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

const valid = `{
  "platforms": [
    { "system": "x86_64-linux", "os": "linux", "arch": "amd64" },
    { "system": "aarch64-darwin", "os": "darwin", "arch": "arm64" }
  ],
  "products": [
    { "name": "terraform", "description": "a product", "license": "bsl11", "resolveLicense": true },
    { "name": "tfc-agent", "description": "a product", "license": "unfree", "resolveLicense": false }
  ]
}`

func TestLoad(t *testing.T) {
	cfg, err := Load(write(t, valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	system, ok := cfg.System("darwin", "arm64")
	if !ok || system != "aarch64-darwin" {
		t.Errorf("System(darwin, arm64) = %q, %t", system, ok)
	}
	if _, ok := cfg.System("windows", "amd64"); ok {
		t.Error("an untracked target resolved to a system")
	}

	product, ok := cfg.Product("terraform")
	if !ok {
		t.Fatal("terraform is not configured")
	}
	// MainProgram defaults to the product name.
	if got := product.Program(); got != "terraform" {
		t.Errorf("Program = %q, want terraform", got)
	}
	if got := product.Tag("1.9.8"); got != "v1.9.8" {
		t.Errorf("Tag = %q, want v1.9.8", got)
	}
}

func TestLoadRejectsInvalidConfigurations(t *testing.T) {
	tests := map[string]string{
		"no platforms": `{"platforms": [], "products": [
			{"name": "terraform", "description": "a product", "license": "bsl11"}]}`,

		"no products": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"}], "products": []}`,

		"duplicate system": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"},
			{"system": "x86_64-linux", "os": "linux", "arch": "386"}], "products": [
			{"name": "terraform", "description": "a product", "license": "bsl11"}]}`,

		// Two systems claiming one target would make System() ambiguous.
		"duplicate target": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"},
			{"system": "x86_64-freebsd", "os": "linux", "arch": "amd64"}], "products": [
			{"name": "terraform", "description": "a product", "license": "bsl11"}]}`,

		"duplicate product": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"}], "products": [
			{"name": "terraform", "description": "a product", "license": "bsl11"},
			{"name": "terraform", "description": "a product", "license": "bsl11"}]}`,

		"product with no licence": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"}], "products": [
			{"name": "terraform", "description": "a product"}]}`,

		"unknown field": `{"platforms": [
			{"system": "x86_64-linux", "os": "linux", "arch": "amd64"}], "products": [
			{"name": "terraform", "description": "a product", "license": "bsl11", "licence": "mpl20"}]}`,
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(write(t, contents)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// The checked-in configuration is what the scheduled workflow runs against.
func TestRepositoryConfigurationIsValid(t *testing.T) {
	cfg, err := Load("../../products.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Products) == 0 {
		t.Fatal("no products configured")
	}

	for _, product := range cfg.Products {
		switch product.License {
		case "mpl20", "bsl11", "unfree":
		default:
			t.Errorf("%s has licence %q, which lib.licenses has no attribute for",
				product.Name, product.License)
		}
	}
}

func TestTagTemplate(t *testing.T) {
	product := Product{Name: "example", TagTemplate: "release-{version}"}

	if got := product.Tag("1.2.3"); got != "release-1.2.3" {
		t.Errorf("Tag = %q, want release-1.2.3", got)
	}
}
