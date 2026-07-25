# `nix-hashicorp-tools`

This repository contains a Nix overlay, with every published release of the main HashiCorp tools, installed from HashiCorp's own official binaries rather than built from source.

This is useful because nixpkgs carries a single version of each of these tools and builds from source.
Infrastructure repositories often need an exact version, matching what CI and the rest of the team run, and sometimes prefer to use the vendor-built binaries which can be verified against supplied checksums.

## Usage

### As a flake

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    hashicorp-tools = {
      url = "github:jen20/nix-hashicorp-tools";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { nixpkgs, hashicorp-tools, ... }:
    let
      pkgs = import nixpkgs {
        system = "aarch64-darwin";
        overlays = [ hashicorp-tools.overlays.default ];
        config.allowUnfree = true;   # see "Licensing" below
      };
    in {
      devShells.aarch64-darwin.default = pkgs.mkShell {
        packages = [
          pkgs.hashicorp.terraform."1.9.8"
          pkgs.hashicorp.vault.latest
          pkgs.hashicorp.consul."1.20.1"
        ];
      };
    };
}
```

`nix flake init -t github:jen20/nix-hashicorp-tools` writes a working version of the above.

### Without flakes

```nix
let
  hashicorp = import (fetchTarball "https://github.com/jen20/nix-hashicorp-tools/archive/main.tar.gz") { };
in
hashicorp.terraform."1.9.8"
```

### From the command line

```console
$ nix run github:jen20/nix-hashicorp-tools#vault -- version
$ nix build 'github:jen20/nix-hashicorp-tools#legacyPackages.aarch64-darwin.terraform."1.9.8"'
```

## The attribute tree

Each product exposes every release that has a build for the system being evaluated, plus these attributes:

| Attribute | Meaning |
|---|---|
| `<version>` | That exact release, e.g. `terraform."1.9.8"` |
| `latest` | The newest release that is not a prerelease |
| `latestPrerelease` | The newest prerelease, where one exists |
| `versions` | Every available version, as a list of strings, oldest first |
| `product` | The product name |

`latest` is resolved **per system**, because the newest release of a product does not necessarily build for every system an older one did. On `armv7l-linux`, `hashicorp.levant.latest` is 0.3.3 rather than 0.4.0, because 0.4.0 dropped 32-bit ARM.

A product is only present on systems where it has at least one release, so `hashicorp.tfc-agent` does not exist on Darwin at all, rather than existing and failing to build.

### Products

Core: `terraform`, `vault`, `consul`, `nomad`, `packer`, `boundary`, `waypoint`

Companions: `terraform-ls`, `consul-template`, `envconsul`, `nomad-pack`, `nomad-autoscaler`, `levant`, `sentinel`, `serf`, `hcdiag`, `hcp`, `tfc-agent`, `vault-radar`, `vlt`

Terraform providers, Packer plugins, Vault plugins and Kubernetes sidecars are out of scope.
Vagrant is excluded because it is not distributed as a plain archive of Go binaries.

Prereleases are addressable (`terraform."1.16.0-beta1"`) but never resolve as `latest`.
Enterprise (`+ent`) builds are excluded.

### Systems

`x86_64-linux`, `aarch64-linux`, `armv7l-linux`, `i686-linux`, `powerpc64le-linux`, `x86_64-darwin`, `aarch64-darwin`

## Licensing

Terraform, Vault, Consul, Nomad, Packer and Boundary moved from MPL-2.0 to BUSL-1.1 during 2023, which nixpkgs classifies as unfree.
Sentinel, `tfc-agent`, `vault-radar` and `vlt` ship under an EULA and are marked `unfree` too.

Everything else is MPL-2.0.

Which of those needs `allowUnfree` depends on how you consume the overlay:

- **Through `overlays.default`** — the packages are built against *your* nixpkgs, so your configuration governs, and BUSL releases need `config.allowUnfree = true` or an `allowUnfreePredicate`. This is the same gate nixpkgs' own `terraform` (for example) sits behind.
- **Through this flake's own outputs** (`nix build .#terraform`, `nix run .#vault`, `legacyPackages`) — unfree is already allowed. Asking a flake whose entire purpose is HashiCorp's official binaries for one of them is the opt-in. `meta.license` is still accurate, so anything you compose these into sees the real licence.

`meta.license` is recorded **per release**, read from the `LICENSE` file at that release's tag. This matters more than it sounds: HashiCorp relicensed maintenance branches at different points, so the licence is not monotonic in version order. Consul 1.15.9 and 1.15.10 are BUSL, while the *later* 1.16.0 through 1.16.4 are MPL. A version cut-off could not express that.

Only redistribution metadata lives here — this repository contains no HashiCorp code, just the URLs and checksums needed to fetch the official archives. Your use of the binaries is governed by HashiCorp's licence terms.

## Updating

The `update` workflow runs daily, refreshes the manifests, verifies them, builds the newest release of every product, and commits to `main`.

To run it by hand:

```console
$ nix develop
$ go run ./cmd/hcnix update              # refresh manifests
$ go run ./cmd/hcnix update -dry-run     # report what would change
$ go run ./cmd/hcnix update -product vault
$ go run ./cmd/hcnix check               # validate the manifests offline
$ go run ./cmd/hcnix list                # summarise the tracked products
```

The updater is built on [go-hashicorp-releases-client][client].

### Signature verification

Checksum files are verified against HashiCorp's published OpenPGP key in `keys/`.
The default `-signatures lenient` policy reports how many releases that affects and continues; a bad or revoked signature is always fatal. `-signatures strict` refuses anything unverifiable, and `-signatures off` skips the check.

## Development

```console
$ nix develop
$ go test ./...
$ nix flake check          # evaluates every release, builds a representative few
```

`nix flake check` includes an `eval-all` check that forces the derivation of every release for the current system. It downloads nothing, but catches a manifest that Nix cannot turn into a valid derivation.

## Licence

MPL-2.0. See [LICENSE](LICENSE).

[rust-overlay]: https://github.com/oxalica/rust-overlay
[zig-overlay]: https://github.com/mitchellh/zig-overlay
[client]: https://github.com/jen20/go-hashicorp-releases-client
