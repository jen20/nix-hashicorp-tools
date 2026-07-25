// Command hcnix regenerates the release manifests that the nix-hashicorp-tools
// overlay is built from.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	releases "github.com/jen20/go-hashicorp-releases-client"

	"github.com/jen20/nix-hashicorp-tools/internal/config"
	"github.com/jen20/nix-hashicorp-tools/internal/update"
)

const userAgent = "nix-hashicorp-tools (+https://github.com/jen20/nix-hashicorp-tools)"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "hcnix: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	command, args := args[0], args[1:]
	switch command {
	case "update":
		return runUpdate(ctx, args)
	case "check":
		return runCheck(args)
	case "list":
		return runList(args)
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `hcnix regenerates the nix-hashicorp-tools release manifests.

Usage:
  hcnix update [flags]   refresh manifests from the HashiCorp releases API
  hcnix check  [flags]   validate the generated manifests offline
  hcnix list   [flags]   summarise the tracked products

Run "hcnix <command> -h" for the flags of a command.
`)
}

// commonFlags are accepted by every subcommand.
type commonFlags struct {
	config  string
	dataDir string
	verbose bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "products.json", "path to the product configuration")
	fs.StringVar(&c.dataDir, "data", "data", "directory holding the generated manifests")
	fs.BoolVar(&c.verbose, "v", false, "log debug detail")
}

func (c *commonFlags) logger() *slog.Logger {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func runUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)

	var common commonFlags
	common.register(fs)

	var (
		products    stringList
		keysDir     = fs.String("keys", "keys", "directory of OpenPGP public keys")
		signatures  = fs.String("signatures", "lenient", "signature policy: off, lenient or strict")
		concurrency = fs.Int("concurrency", 8, "releases to fetch at once")
		dryRun      = fs.Bool("dry-run", false, "report what would change without writing")
		timeout     = fs.Duration("timeout", 30*time.Minute, "overall time limit")
	)
	fs.Var(&products, "product", "restrict the run to a product (repeatable)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(common.config)
	if err != nil {
		return err
	}

	names := []string(products)
	if len(names) == 0 {
		names = cfg.Names()
	}

	logger := common.logger()

	httpClient := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: &update.Transport{UserAgent: userAgent, Attempts: 4},
	}
	// The releases client issues its listing requests through
	// http.DefaultClient, so retries and the User-Agent only reach them if
	// the default transport is replaced too.
	http.DefaultClient.Transport = httpClient.Transport

	client, err := releases.New(
		releases.WithHTTPClient(httpClient),
		releases.WithUserAgent(userAgent),
	)
	if err != nil {
		return err
	}

	opts := update.Options{
		DataDir:     common.dataDir,
		KeysDir:     *keysDir,
		Concurrency: *concurrency,
		DryRun:      *dryRun,
		Logger:      logger,
	}

	switch *signatures {
	case "off":
	case "strict":
		opts.StrictSignatures = true
		fallthrough
	case "lenient":
		verifier, err := update.NewVerifier(ctx, *keysDir)
		if err != nil {
			return fmt.Errorf("preparing signature verification: %w", err)
		}
		defer func() {
			_ = verifier.Close()
		}()
		opts.Verifier = verifier
	default:
		return fmt.Errorf("unknown signature policy %q", *signatures)
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	result, err := update.New(cfg, client, httpClient, opts).Run(ctx, names)
	if err != nil {
		return err
	}

	report(result, *dryRun)

	return nil
}

func report(result update.Result, dryRun bool) {
	var added, removed, unverified, skipped, unavailable int

	for _, product := range result.Products {
		added += len(product.Added)
		removed += len(product.Removed)
		unverified += product.Unverified
		skipped += product.Skipped
		unavailable += product.Unavailable

		if len(product.Added) == 0 && len(product.Removed) == 0 {
			continue
		}

		fmt.Printf("%-18s %d releases", product.Product, product.Total)
		if len(product.Added) > 0 {
			fmt.Printf("  +%s", strings.Join(product.Added, " +"))
		}
		if len(product.Removed) > 0 {
			fmt.Printf("  -%s", strings.Join(product.Removed, " -"))
		}
		fmt.Println()
	}

	verb := "added"
	if dryRun {
		verb = "would add"
	}

	fmt.Printf("\n%s %d releases, removed %d, across %d products\n",
		verb, added, removed, len(result.Products))

	if unverified > 0 {
		// Every release predating HashiCorp's April 2021 signing key
		// rotation lands here: the key that signed them was revoked and is
		// no longer published.
		fmt.Printf("%d checksum files could not be attributed to a trusted key\n", unverified)
	}
	if skipped > 0 {
		fmt.Printf("%d builds were skipped for a non-canonical URL or missing checksum\n", skipped)
	}
	if unavailable > 0 {
		// Standing coverage gap rather than something this run did: these are
		// releases built only for untracked targets, or whose artefacts do
		// not follow the canonical layout. Reporting the total each run keeps
		// it from reading as full coverage.
		fmt.Printf("%d releases offer nothing to any tracked platform (see \"unavailable\" in the manifests)\n",
			unavailable)
	}
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)

	var common commonFlags
	common.register(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(common.config)
	if err != nil {
		return err
	}

	problems, checked, err := update.Check(cfg, common.dataDir)
	if err != nil {
		return err
	}

	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problems in the generated manifests", len(problems))
	}

	fmt.Printf("checked %d releases across %d products\n", checked, len(cfg.Products))

	return nil
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)

	var common commonFlags
	common.register(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(common.config)
	if err != nil {
		return err
	}

	return update.List(cfg, common.dataDir, os.Stdout)
}

// stringList collects a repeatable string flag.
type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	if value == "" {
		return errors.New("empty value")
	}
	*s = append(*s, value)
	return nil
}
