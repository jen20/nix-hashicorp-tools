package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	releases "github.com/jen20/go-hashicorp-releases-client"

	"github.com/jen20/nix-hashicorp-tools/internal/config"
	"github.com/jen20/nix-hashicorp-tools/internal/manifest"
)

// build describes one artefact the fake releases API should publish.
type build struct {
	os, arch string
	// url overrides the canonical download URL, to exercise the guard
	// against the layout the overlay derives in Nix.
	url string
	// noChecksum omits the artefact from the release's SHA256SUMS file.
	noChecksum bool
}

type release struct {
	version    string
	prerelease bool
	withdrawn  bool
	builds     []build
}

// fakeAPI serves just enough of the releases API and the download host for the
// updater to run against.
type fakeAPI struct {
	product  string
	releases []release
	server   *httptest.Server
}

func newFakeAPI(t *testing.T, product string, list []release) *fakeAPI {
	t.Helper()

	api := &fakeAPI{product: product, releases: list}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/releases/{product}", api.handleList)
	mux.HandleFunc("/sums/{version}", api.handleSums)

	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)

	return api
}

func (a *fakeAPI) handleList(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("product") != a.product {
		http.Error(w, "no such product", http.StatusNotFound)
		return
	}

	// The client pages with an "after" cursor and stops on an empty page.
	var page []releases.ReleaseInfo
	if r.URL.Query().Get("after") == "" {
		for i, rel := range a.releases {
			page = append(page, a.info(rel, i))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

func (a *fakeAPI) info(rel release, index int) releases.ReleaseInfo {
	info := releases.ReleaseInfo{
		Name:                a.product,
		Version:             rel.version,
		IsPrerelease:        rel.prerelease,
		LicenseClass:        *releases.LicenseClassOSS,
		TimestampCreated:    time.Unix(int64(index)*86400, 0).UTC(),
		URLSHASUMs:          a.server.URL + "/sums/" + rel.version,
		URLProjectWebsite:   "https://example.invalid/" + a.product,
		URLSourceRepository: "https://github.com/hashicorp/" + a.product,
		Status:              releases.ReleaseStatus{State: releases.ReleaseStateSupported},
	}
	if rel.withdrawn {
		info.Status = releases.ReleaseStatus{State: releases.ReleaseStateWithdrawn, Message: "withdrawn"}
	}

	for _, b := range rel.builds {
		url := b.url
		if url == "" {
			url = ArchiveURL(a.product, rel.version, b.os, b.arch)
		}
		info.Builds = append(info.Builds, releases.BuildInfo{OS: b.os, Arch: b.arch, URL: url})
	}

	return info
}

func (a *fakeAPI) handleSums(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")

	for _, rel := range a.releases {
		if rel.version != version {
			continue
		}

		var sums strings.Builder
		for _, b := range rel.builds {
			if b.noChecksum {
				continue
			}
			name := ArchiveName(a.product, rel.version, b.os, b.arch)
			fmt.Fprintf(&sums, "%s  %s\n", fakeSum(name), name)
		}

		_, _ = w.Write([]byte(sums.String()))
		return
	}

	http.Error(w, "no such release", http.StatusNotFound)
}

// fakeSum derives a deterministic, well-formed SHA-256 for an artefact name.
func fakeSum(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

func testConfig(product string) *config.Config {
	return &config.Config{
		Platforms: []config.Platform{
			{System: "x86_64-linux", OS: "linux", Arch: "amd64"},
			{System: "aarch64-darwin", OS: "darwin", Arch: "arm64"},
			{System: "armv7l-linux", OS: "linux", Arch: "arm"},
		},
		Products: []config.Product{{
			Name:        product,
			Description: "a product",
			License:     "mpl20",
			// Resolving licences would reach GitHub, which a unit test
			// has no business doing; DetectLicense is covered separately.
			ResolveLicense: false,
		}},
	}
}

func runUpdater(t *testing.T, api *fakeAPI, cfg *config.Config, dataDir string) update {
	t.Helper()

	client, err := releases.New(releases.WithBaseURL(api.server.URL))
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	updater := New(cfg, client, api.server.Client(), Options{
		DataDir:     dataDir,
		Concurrency: 4,
	})

	result, err := updater.Run(context.Background(), cfg.Names())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	m, err := manifest.Read(manifest.ProductPath(dataDir, cfg.Products[0].Name))
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}

	return update{result: result, manifest: m}
}

type update struct {
	result   Result
	manifest *manifest.Manifest
}

var amd64Linux = build{os: "linux", arch: "amd64"}
var arm64Darwin = build{os: "darwin", arch: "arm64"}
var armLinux = build{os: "linux", arch: "arm"}

func TestUpdate(t *testing.T) {
	dataDir := t.TempDir()

	api := newFakeAPI(t, "levant", []release{
		{version: "0.3.0-beta1", prerelease: true, builds: []build{amd64Linux, armLinux}},
		{version: "0.3.3", builds: []build{amd64Linux, armLinux, arm64Darwin}},
		// The newest release dropped 32-bit ARM.
		{version: "0.4.0", builds: []build{amd64Linux, arm64Darwin}},
	})

	got := runUpdater(t, api, testConfig("levant"), dataDir)

	if len(got.manifest.Versions) != 3 {
		t.Fatalf("recorded %d releases, want 3", len(got.manifest.Versions))
	}

	wantOrder := []string{"0.3.0-beta1", "0.3.3", "0.4.0"}
	if !reflect.DeepEqual(got.manifest.Order, wantOrder) {
		t.Errorf("order = %v, want %v", got.manifest.Order, wantOrder)
	}

	// The per-system pointer for a system the newest release abandoned has to
	// fall back to the newest release that still builds for it.
	wantLatest := map[string]string{
		"x86_64-linux":   "0.4.0",
		"aarch64-darwin": "0.4.0",
		"armv7l-linux":   "0.3.3",
	}
	if !reflect.DeepEqual(got.manifest.Latest, wantLatest) {
		t.Errorf("latest = %v, want %v", got.manifest.Latest, wantLatest)
	}

	if got.manifest.Homepage != "https://example.invalid/levant" {
		t.Errorf("homepage = %q", got.manifest.Homepage)
	}

	wantSum := fakeSum("levant_0.4.0_linux_amd64.zip")
	if sum := got.manifest.Versions["0.4.0"].Platforms["x86_64-linux"]; sum != wantSum {
		t.Errorf("recorded hash = %q, want %q", sum, wantSum)
	}

	// The index and platform map have to agree with the manifests, since Nix
	// enumerates the tree from them without reading the manifests.
	problems, checked, err := Check(testConfig("levant"), dataDir)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(problems) > 0 {
		t.Errorf("Check reported problems: %v", problems)
	}
	if checked != 3 {
		t.Errorf("Check saw %d releases, want 3", checked)
	}
}

func TestUpdateSkipsUnusableBuilds(t *testing.T) {
	dataDir := t.TempDir()

	api := newFakeAPI(t, "terraform", []release{
		{version: "1.0.0", builds: []build{
			amd64Linux,
			// A doubled filename prefix, as some 2019 Terraform alphas
			// publish. Nix derives the canonical URL, so recording this
			// release's hash under the derived URL would 404 at build time.
			{
				os:   "darwin",
				arch: "arm64",
				url:  "https://releases.hashicorp.com/terraform/1.0.0/terraform_1.0.0_terraform_1.0.0_darwin_arm64.zip",
			},
			// Listed as a build but absent from SHA256SUMS, as some early
			// Waypoint releases are.
			{os: "linux", arch: "arm", noChecksum: true},
		}},
	})

	got := runUpdater(t, api, testConfig("terraform"), dataDir)

	platforms := got.manifest.Versions["1.0.0"].Platforms
	if len(platforms) != 1 {
		t.Fatalf("recorded platforms %v, want only x86_64-linux", platforms)
	}
	if _, ok := platforms["x86_64-linux"]; !ok {
		t.Errorf("recorded platforms %v, want x86_64-linux", platforms)
	}

	// A skipped build must be reported rather than silently dropped.
	if skipped := got.result.Products[0].Skipped; skipped != 2 {
		t.Errorf("reported %d skipped builds, want 2", skipped)
	}
}

// Releases with nothing to offer never land in Versions, so unless they are
// remembered they are re-fetched on every run for ever.
func TestUpdateRemembersUnusableReleases(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("waypoint")

	api := newFakeAPI(t, "waypoint", []release{
		// Listed as a build but absent from SHA256SUMS, alongside an
		// untracked target that is present, so the checksum file is not empty.
		{version: "0.1.0", builds: []build{
			{os: "linux", arch: "amd64", noChecksum: true},
			{os: "windows", arch: "amd64"},
		}},
		// Built only for targets the overlay does not track.
		{version: "0.1.1", builds: []build{{os: "windows", arch: "amd64"}}},
		{version: "0.2.0", builds: []build{amd64Linux}},
	})

	first := runUpdater(t, api, cfg, dataDir)

	want := []string{"0.1.0", "0.1.1"}
	if !reflect.DeepEqual(first.manifest.Unavailable, want) {
		t.Errorf("unavailable = %v, want %v", first.manifest.Unavailable, want)
	}
	if len(first.manifest.Versions) != 1 {
		t.Errorf("recorded %d releases, want 1", len(first.manifest.Versions))
	}

	// A second run must not re-examine them, and must not lose them either.
	second := runUpdater(t, api, cfg, dataDir)

	if second.result.Changed {
		t.Error("a second identical run reported a change")
	}
	if skipped := second.result.Products[0].Skipped; skipped != 0 {
		t.Errorf("a second run re-examined %d builds, want 0", skipped)
	}
	if !reflect.DeepEqual(second.manifest.Unavailable, want) {
		t.Errorf("a second run recorded unavailable = %v, want %v", second.manifest.Unavailable, want)
	}

	problems, _, err := Check(cfg, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Errorf("Check reported problems: %v", problems)
	}
}

func TestUpdateSkipsWithdrawnReleases(t *testing.T) {
	dataDir := t.TempDir()

	api := newFakeAPI(t, "consul", []release{
		{version: "1.0.0", builds: []build{amd64Linux}},
		{version: "1.0.1", withdrawn: true, builds: []build{amd64Linux}},
	})

	got := runUpdater(t, api, testConfig("consul"), dataDir)

	if _, ok := got.manifest.Versions["1.0.1"]; ok {
		t.Error("a withdrawn release was recorded")
	}
	if got.manifest.Latest["x86_64-linux"] != "1.0.0" {
		t.Errorf("latest = %v, want 1.0.0", got.manifest.Latest)
	}
}

// A run with nothing new to record must leave the files untouched, so that the
// scheduled workflow does not produce an empty commit every day.
func TestUpdateIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("packer")

	api := newFakeAPI(t, "packer", []release{
		{version: "1.10.0", builds: []build{amd64Linux, arm64Darwin}},
		{version: "1.11.0", builds: []build{amd64Linux, arm64Darwin}},
	})

	first := runUpdater(t, api, cfg, dataDir)
	if !first.result.Changed {
		t.Fatal("the first run reported no change")
	}

	second := runUpdater(t, api, cfg, dataDir)
	if second.result.Changed {
		t.Error("a second identical run reported a change")
	}
	if len(second.result.Products[0].Added) != 0 {
		t.Errorf("a second run re-added %v", second.result.Products[0].Added)
	}
	if !reflect.DeepEqual(first.manifest, second.manifest) {
		t.Error("a second run produced a different manifest")
	}
}

// An empty response must not be allowed to quietly erase a populated manifest.
func TestUpdateRefusesToEmptyAManifest(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("nomad")

	populated := newFakeAPI(t, "nomad", []release{
		{version: "1.0.0", builds: []build{amd64Linux}},
	})
	runUpdater(t, populated, cfg, dataDir)

	empty := newFakeAPI(t, "nomad", nil)
	client, err := releases.New(releases.WithBaseURL(empty.server.URL))
	if err != nil {
		t.Fatal(err)
	}

	updater := New(cfg, client, empty.server.Client(), Options{DataDir: dataDir, Concurrency: 1})
	if _, err := updater.Run(context.Background(), cfg.Names()); err == nil {
		t.Fatal("expected an error")
	}

	m, err := manifest.Read(manifest.ProductPath(dataDir, "nomad"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Versions) != 1 {
		t.Errorf("the manifest now holds %d releases, want 1", len(m.Versions))
	}
}

func TestCheckDetectsATamperedManifest(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("vault")

	api := newFakeAPI(t, "vault", []release{
		{version: "1.0.0", builds: []build{amd64Linux}},
		{version: "1.1.0", builds: []build{amd64Linux}},
	})
	runUpdater(t, api, cfg, dataDir)

	path := manifest.ProductPath(dataDir, "vault")
	m, err := manifest.Read(path)
	if err != nil {
		t.Fatal(err)
	}

	// Point latest at a release that does not exist, as a hand-edit or a
	// bad merge might.
	m.Latest["x86_64-linux"] = "9.9.9"
	if _, err := manifest.Write(path, m); err != nil {
		t.Fatal(err)
	}

	problems, _, err := Check(cfg, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Error("Check accepted a manifest whose latest pointer dangles")
	}
}

func TestCheckDetectsAMissingManifest(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("boundary")

	api := newFakeAPI(t, "boundary", []release{
		{version: "0.14.0", builds: []build{amd64Linux}},
	})
	runUpdater(t, api, cfg, dataDir)

	// Configure a second product that was never fetched.
	cfg.Products = append(cfg.Products, config.Product{
		Name:        "waypoint",
		Description: "a product",
		License:     "mpl20",
	})

	problems, _, err := Check(cfg, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, problem := range problems {
		if strings.Contains(problem, "waypoint") {
			found = true
		}
	}
	if !found {
		t.Errorf("Check did not report the missing manifest: %v", problems)
	}
}

func TestUpdateWritesTheIndexAndPlatformMap(t *testing.T) {
	dataDir := t.TempDir()
	cfg := testConfig("hcp")

	api := newFakeAPI(t, "hcp", []release{
		{version: "0.11.0", builds: []build{amd64Linux, arm64Darwin}},
	})
	got := runUpdater(t, api, cfg, dataDir)

	index, err := readJSON[manifest.Index](filepath.Join(dataDir, manifest.IndexFile))
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	if !reflect.DeepEqual(index["hcp"].Latest, got.manifest.Latest) {
		t.Errorf("index latest = %v, want %v", index["hcp"].Latest, got.manifest.Latest)
	}

	platforms, err := readJSON[manifest.Platforms](filepath.Join(dataDir, manifest.PlatformsFile))
	if err != nil {
		t.Fatalf("reading platforms: %v", err)
	}
	want := manifest.Target{OS: "darwin", Arch: "arm64"}
	if platforms["aarch64-darwin"] != want {
		t.Errorf("aarch64-darwin = %v, want %v", platforms["aarch64-darwin"], want)
	}
}
