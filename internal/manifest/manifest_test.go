package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFinalise(t *testing.T) {
	m := &Manifest{
		Product: "levant",
		Versions: map[string]Release{
			"0.3.0-beta1": {
				Prerelease: true,
				Platforms:  map[string]string{"x86_64-linux": "a", "armv7l-linux": "b"},
			},
			"0.3.3": {
				Platforms: map[string]string{"x86_64-linux": "c", "armv7l-linux": "d"},
			},
			// The newest release dropped 32-bit ARM.
			"0.4.0": {
				Platforms: map[string]string{"x86_64-linux": "e"},
			},
			"0.10.0": {
				Platforms: map[string]string{"x86_64-linux": "f"},
			},
		},
	}

	m.Finalise()

	wantOrder := []string{"0.3.0-beta1", "0.3.3", "0.4.0", "0.10.0"}
	if !reflect.DeepEqual(m.Order, wantOrder) {
		t.Errorf("order = %v, want %v", m.Order, wantOrder)
	}

	// A system that the newest release no longer builds for must still point
	// at the newest release that does.
	wantLatest := map[string]string{"x86_64-linux": "0.10.0", "armv7l-linux": "0.3.3"}
	if !reflect.DeepEqual(m.Latest, wantLatest) {
		t.Errorf("latest = %v, want %v", m.Latest, wantLatest)
	}

	wantPrerelease := map[string]string{"x86_64-linux": "0.3.0-beta1", "armv7l-linux": "0.3.0-beta1"}
	if !reflect.DeepEqual(m.LatestPrerelease, wantPrerelease) {
		t.Errorf("latestPrerelease = %v, want %v", m.LatestPrerelease, wantPrerelease)
	}
}

func TestFinaliseOmitsEmptyPrereleasePointers(t *testing.T) {
	m := &Manifest{
		Versions: map[string]Release{
			"1.0.0": {Platforms: map[string]string{"x86_64-linux": "a"}},
		},
	}

	m.Finalise()

	if m.LatestPrerelease != nil {
		t.Errorf("latestPrerelease = %v, want nil", m.LatestPrerelease)
	}
}

func TestMarshalIsStable(t *testing.T) {
	m := &Manifest{
		Product:     "terraform",
		MainProgram: "terraform",
		Versions: map[string]Release{
			"1.9.8":  {License: "bsl11", Platforms: map[string]string{"x86_64-linux": "a", "aarch64-darwin": "b"}},
			"1.10.0": {License: "bsl11", Platforms: map[string]string{"x86_64-linux": "c"}},
		},
	}
	m.Finalise()

	first, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	for range 5 {
		again, err := Marshal(m)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(again) != string(first) {
			t.Fatal("marshalling the same manifest twice produced different output")
		}
	}

	if first[len(first)-1] != '\n' {
		t.Error("output does not end in a newline")
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products", "terraform.json")

	// A manifest that does not exist yet must read as an empty one so that a
	// first run and an incremental run take the same path.
	empty, err := Read(path)
	if err != nil {
		t.Fatalf("Read of a missing manifest: %v", err)
	}
	if len(empty.Versions) != 0 {
		t.Fatalf("missing manifest read as %d versions", len(empty.Versions))
	}

	original := &Manifest{
		Product:     "terraform",
		MainProgram: "terraform",
		Versions: map[string]Release{
			"1.9.8": {License: "bsl11", Platforms: map[string]string{"x86_64-linux": "a"}},
		},
	}
	original.Finalise()

	changed, err := Write(path, original)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !changed {
		t.Error("writing a new file reported no change")
	}

	// Rewriting identical content must not touch the file, so that an update
	// run with nothing to do produces no commit.
	changed, err = Write(path, original)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if changed {
		t.Error("rewriting identical content reported a change")
	}

	loaded, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(loaded, original) {
		t.Errorf("round trip changed the manifest:\n got %+v\nwant %+v", loaded, original)
	}
}

func TestWriteLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")

	if _, err := Write(path, Index{"terraform": {Latest: map[string]string{"x86_64-linux": "1.9.8"}}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "index.json" {
		t.Errorf("directory contains %v, want only index.json", entries)
	}

	var index Index
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &index); err != nil {
		t.Fatalf("written index does not parse: %v", err)
	}
}
