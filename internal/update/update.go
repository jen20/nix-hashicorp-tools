// Package update refreshes the generated release manifests from the HashiCorp
// releases API.
package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"sync"

	releases "github.com/jen20/go-hashicorp-releases-client"

	"github.com/jen20/nix-hashicorp-tools/internal/config"
	"github.com/jen20/nix-hashicorp-tools/internal/manifest"
	"github.com/jen20/nix-hashicorp-tools/internal/semver"
)

// Options configures an Updater.
type Options struct {
	// DataDir is the root beneath which manifests are written.
	DataDir string

	// KeysDir holds the OpenPGP public keys used to verify checksum files.
	KeysDir string

	// Concurrency bounds the number of releases fetched at once.
	Concurrency int

	// DryRun reports what would change without writing anything.
	DryRun bool

	// Verifier checks the signature over each release's checksum file. A nil
	// Verifier disables signature checking.
	Verifier *Verifier

	// StrictSignatures makes a signature that cannot be attributed to a
	// trusted key fatal rather than merely reported.
	StrictSignatures bool

	Logger *slog.Logger
}

// Updater refreshes manifests.
type Updater struct {
	cfg    *config.Config
	client *releases.Client
	http   *http.Client
	opts   Options
}

// New creates an Updater.
func New(cfg *config.Config, client *releases.Client, httpClient *http.Client, opts Options) *Updater {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Updater{cfg: cfg, client: client, http: httpClient, opts: opts}
}

// ProductResult summarises what changed for one product.
type ProductResult struct {
	Product string
	Added   []string
	Removed []string
	Total   int

	// Unverified counts releases whose checksum file could not be attributed
	// to a trusted key.
	Unverified int

	// Skipped counts individual builds left out because their download URL
	// deviated from the canonical layout or had no published checksum.
	// Reporting this keeps a coverage gap from reading as full coverage.
	Skipped int

	// Unavailable counts releases with nothing to offer any tracked platform.
	Unavailable int

	Changed bool
}

// Result summarises a whole run.
type Result struct {
	Products []ProductResult
	Changed  bool
}

// Run refreshes the named products, then rewrites the index and platform map.
// Products not named are left alone, but still contribute their existing
// manifests to the index.
func (u *Updater) Run(ctx context.Context, names []string) (Result, error) {
	var result Result

	selected := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := u.cfg.Product(name); !ok {
			return result, fmt.Errorf("product %q is not configured", name)
		}
		selected[name] = struct{}{}
	}

	index := manifest.Index{}

	for _, product := range u.cfg.Products {
		path := manifest.ProductPath(u.opts.DataDir, product.Name)

		if _, ok := selected[product.Name]; !ok {
			existing, err := manifest.Read(path)
			if err != nil {
				return result, err
			}
			if len(existing.Versions) > 0 {
				index[product.Name] = existing.Entry()
			}
			continue
		}

		updated, productResult, err := u.product(ctx, product)
		if err != nil {
			return result, fmt.Errorf("%s: %w", product.Name, err)
		}

		if !u.opts.DryRun {
			changed, err := manifest.Write(path, updated)
			if err != nil {
				return result, fmt.Errorf("%s: %w", product.Name, err)
			}
			productResult.Changed = changed
		}

		index[product.Name] = updated.Entry()
		result.Products = append(result.Products, productResult)
		result.Changed = result.Changed || productResult.Changed
	}

	if u.opts.DryRun {
		return result, nil
	}

	platforms := manifest.Platforms{}
	for _, platform := range u.cfg.Platforms {
		platforms[platform.System] = manifest.Target{OS: platform.OS, Arch: platform.Arch}
	}

	for path, value := range map[string]any{
		filepath.Join(u.opts.DataDir, manifest.IndexFile):     index,
		filepath.Join(u.opts.DataDir, manifest.PlatformsFile): platforms,
	} {
		changed, err := manifest.Write(path, value)
		if err != nil {
			return result, err
		}
		result.Changed = result.Changed || changed
	}

	return result, nil
}

// product rebuilds one product's manifest.
func (u *Updater) product(ctx context.Context, product config.Product) (*manifest.Manifest, ProductResult, error) {
	result := ProductResult{Product: product.Name}

	path := manifest.ProductPath(u.opts.DataDir, product.Name)
	existing, err := manifest.Read(path)
	if err != nil {
		return nil, result, err
	}

	available, err := u.list(ctx, product.Name)
	if err != nil {
		return nil, result, err
	}
	if len(available) == 0 {
		// A product that has been published before does not stop having
		// releases. Treating this as an error stops a bad API response from
		// silently emptying a manifest.
		if len(existing.Versions) > 0 {
			return nil, result, errors.New("API returned no releases but the manifest is not empty")
		}
		return nil, result, errors.New("API returned no releases")
	}

	updated := &manifest.Manifest{
		Product:     product.Name,
		Description: product.Description,
		MainProgram: product.Program(),
		Versions:    make(map[string]manifest.Release, len(available)),
	}

	// The newest release carries the most current project metadata.
	newest := available[len(available)-1]
	updated.Homepage = newest.URLProjectWebsite
	updated.SourceRepository = newest.URLSourceRepository

	unusable := make(map[string]struct{}, len(existing.Unavailable))
	for _, version := range existing.Unavailable {
		unusable[version] = struct{}{}
	}

	var (
		mu      sync.Mutex
		pending []releases.ReleaseInfo
	)

	for _, info := range available {
		// A release already in the manifest is immutable: its archives and
		// their hashes cannot change. Reusing it is what makes a routine run
		// cost nothing beyond listing.
		if release, ok := existing.Versions[info.Version]; ok && release.License != "" {
			updated.Versions[info.Version] = release
			continue
		}
		// Likewise for a release already known to have nothing to offer.
		if _, ok := unusable[info.Version]; ok {
			updated.Unavailable = append(updated.Unavailable, info.Version)
			continue
		}
		pending = append(pending, info)
	}

	err = forEach(ctx, pending, u.opts.Concurrency, func(ctx context.Context, info releases.ReleaseInfo) error {
		release, stats, err := u.release(ctx, product, info)
		if err != nil {
			return fmt.Errorf("%s: %w", info.Version, err)
		}

		mu.Lock()
		defer mu.Unlock()

		if stats.unverified {
			result.Unverified++
		}
		result.Skipped += stats.skipped
		// A release with no build for any tracked platform is not an error,
		// it simply has nothing to contribute. Remembering it stops it from
		// being re-examined on every subsequent run.
		if len(release.Platforms) > 0 {
			updated.Versions[info.Version] = release
			result.Added = append(result.Added, info.Version)
		} else {
			updated.Unavailable = append(updated.Unavailable, info.Version)
		}
		return nil
	})
	if err != nil {
		return nil, result, err
	}

	for version := range existing.Versions {
		if _, ok := updated.Versions[version]; !ok {
			result.Removed = append(result.Removed, version)
		}
	}

	sortVersions(result.Added)
	sortVersions(result.Removed)
	sortVersions(updated.Unavailable)

	updated.Finalise()
	result.Total = len(updated.Versions)
	result.Unavailable = len(updated.Unavailable)

	if len(updated.Latest) == 0 {
		return nil, result, errors.New("no stable release builds for any tracked platform")
	}

	return updated, result, nil
}

// list collects every OSS release of a product, oldest first.
func (u *Updater) list(ctx context.Context, product string) ([]releases.ReleaseInfo, error) {
	sequence, err := u.client.Releases(ctx, product, releases.LicenseClassOSS)
	if err != nil {
		return nil, err
	}

	var available []releases.ReleaseInfo
	for info, err := range sequence {
		if err != nil {
			return nil, err
		}
		if info.Status.State == releases.ReleaseStateWithdrawn {
			u.opts.Logger.Warn("skipping withdrawn release",
				"product", product, "version", info.Version, "reason", info.Status.Message)
			continue
		}
		available = append(available, info)
	}

	sort.Slice(available, func(i, j int) bool {
		return semver.Less(available[i].Version, available[j].Version)
	})

	return available, nil
}

// releaseStats records what a single release contributed beyond its entry.
type releaseStats struct {
	unverified bool
	skipped    int
}

// release turns one API release into a manifest entry, fetching and verifying
// its checksums and resolving its licence.
func (u *Updater) release(
	ctx context.Context,
	product config.Product,
	info releases.ReleaseInfo,
) (manifest.Release, releaseStats, error) {
	var stats releaseStats

	release := manifest.Release{
		Prerelease: info.IsPrerelease,
		License:    product.License,
		Platforms:  map[string]string{},
	}

	sumsBody, err := u.get(ctx, info.URLSHASUMs)
	if err != nil {
		return release, stats, fmt.Errorf("fetching checksums: %w", err)
	}

	sums, err := ParseSHASums(sumsBody)
	if err != nil {
		return release, stats, fmt.Errorf("parsing checksums: %w", err)
	}

	if u.opts.Verifier != nil {
		verified, err := u.verify(ctx, info, sumsBody)
		if err != nil {
			return release, stats, err
		}
		stats.unverified = !verified
	}

	for _, build := range info.Builds {
		system, tracked := u.cfg.System(build.OS, build.Arch)
		if !tracked {
			continue
		}

		// Nix derives this URL from the product, version and target rather
		// than storing it, so a deviation must not be recorded as if the
		// derived URL would work. A handful of 2019 Terraform alphas publish
		// a doubled filename prefix and land here.
		if expected := ArchiveURL(product.Name, info.Version, build.OS, build.Arch); build.URL != expected {
			u.opts.Logger.Warn("skipping build with a non-canonical URL",
				"product", product.Name, "version", info.Version,
				"expected", expected, "actual", build.URL)
			stats.skipped++
			continue
		}

		sum, ok := sums[ArchiveName(product.Name, info.Version, build.OS, build.Arch)]
		if !ok {
			u.opts.Logger.Warn("skipping build with no published checksum",
				"product", product.Name, "version", info.Version, "system", system)
			stats.skipped++
			continue
		}

		release.Platforms[system] = sum
	}

	if len(release.Platforms) == 0 {
		return release, stats, nil
	}

	if product.ResolveLicense {
		release.License = u.license(ctx, product, info)
	}

	return release, stats, nil
}

// verify checks the detached signature over a checksum file, reporting whether
// it could be attributed to a trusted key.
func (u *Updater) verify(ctx context.Context, info releases.ReleaseInfo, sums []byte) (bool, error) {
	if len(info.URLSHASUMsSignatures) == 0 {
		if u.opts.StrictSignatures {
			return false, errors.New("release publishes no signature")
		}
		u.opts.Logger.Warn("release publishes no signature",
			"product", info.Name, "version", info.Version)
		return false, nil
	}

	// Any one valid signature is enough; HashiCorp publishes the same
	// signature both with and without a key ID in the filename.
	var lastErr error
	for _, url := range info.URLSHASUMsSignatures {
		signature, err := u.get(ctx, url)
		if err != nil {
			lastErr = err
			continue
		}

		outcome, err := u.opts.Verifier.Verify(ctx, signature, sums)
		if err != nil {
			// A bad or revoked signature is always fatal: it means the
			// checksums do not come from HashiCorp.
			return false, fmt.Errorf("verifying %s: %w", url, err)
		}
		if outcome == SignatureGood {
			return true, nil
		}
		lastErr = errors.New("signed by a key that is not in the keyring")
	}

	if u.opts.StrictSignatures {
		return false, fmt.Errorf("could not verify signature: %w", lastErr)
	}

	// Releases predating HashiCorp's April 2021 key rotation are signed with
	// a key that was revoked and is no longer published, so they can never be
	// verified. Reporting the count is more useful than failing the run.
	u.opts.Logger.Debug("could not verify signature",
		"product", info.Name, "version", info.Version, "reason", lastErr)

	return false, nil
}

// license reads the LICENSE file at a release's source tag, falling back to the
// product's configured licence whenever the tag cannot be read or understood.
//
// This is resolved per release rather than from a version cut-off table
// because HashiCorp relicensed maintenance branches under BUSL at different
// points per branch: Consul 1.15.10 is BUSL while the later 1.16.1 is MPL.
func (u *Updater) license(ctx context.Context, product config.Product, info releases.ReleaseInfo) string {
	if info.URLSourceRepository == "" {
		return product.License
	}

	url, err := rawURL(info.URLSourceRepository, product.Tag(info.Version), "LICENSE")
	if err != nil {
		u.opts.Logger.Warn("cannot resolve licence from source",
			"product", product.Name, "version", info.Version, "error", err)
		return product.License
	}

	body, err := u.get(ctx, url)
	if err != nil {
		// Not every release has a matching public tag; older products
		// predate the current tagging convention entirely.
		u.opts.Logger.Debug("no LICENSE at source tag, using configured licence",
			"product", product.Name, "version", info.Version, "url", url)
		return product.License
	}

	license, ok := DetectLicense(body)
	if !ok {
		u.opts.Logger.Warn("unrecognised licence text, using configured licence",
			"product", product.Name, "version", info.Version, "url", url)
		return product.License
	}

	return license
}

// get retrieves a URL in full.
func (u *Updater) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: http %d", url, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func sortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return semver.Less(versions[i], versions[j])
	})
}

// forEach runs fn over items with at most workers running at once, stopping
// early and returning the first error.
//
// Hand-rolled rather than golang.org/x/sync/errgroup so that the releases
// client stays this module's only dependency.
func forEach[T any](ctx context.Context, items []T, workers int, fn func(context.Context, T) error) error {
	if len(items) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	queue := make(chan T)
	go func() {
		defer close(queue)
		for _, item := range items {
			select {
			case queue <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	var (
		wg    sync.WaitGroup
		once  sync.Once
		first error
	)

	for range min(workers, len(items)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range queue {
				if err := fn(ctx, item); err != nil {
					once.Do(func() {
						first = err
						cancel()
					})
					return
				}
			}
		}()
	}

	wg.Wait()

	return first
}
