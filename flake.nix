{
  description = "nix-hashicorp-tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    systems = {
      url = "github:nix-systems/default";
      flake = false;
    };

    flake-compat = {
      url = "github:edolstra/flake-compat";
      flake = false;
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      systems,
      ...
    }:
    let
      inherit (nixpkgs) lib;

      eachSystem = lib.genAttrs (import systems);

      # Terraform, Vault, Consul, Nomad, Packer and Boundary have been BUSL
      # licensed since 2023, and Sentinel and friends ship under an EULA, all
      # of which nixpkgs classifies as unfree. This flake's own outputs are
      # evaluated with that allowed: asking a flake whose entire purpose is
      # HashiCorp's official binaries for one of them is the opt-in, and every
      # derivation still carries an accurate meta.license.
      #
      # `overlays.default` deliberately does not do this. It builds against the
      # caller's own nixpkgs, so their configuration governs.
      #
      # Setting config here is pure; only NIXPKGS_ALLOW_UNFREE is not.
      pkgsFor = eachSystem (
        system:
        import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        }
      );

      treeFor = lib.mapAttrs (
        system: pkgs:
        import ./lib/packages.nix {
          inherit pkgs system;
          inherit (pkgs) lib;
        }
      ) pkgsFor;

      productsFor = system: lib.attrNames treeFor.${system};

      hcnixFor = lib.mapAttrs (
        _: pkgs:
        pkgs.buildGoModule {
          pname = "hcnix";
          version = "0.1.0";

          src = lib.fileset.toSource {
            root = ./.;
            fileset = lib.fileset.unions [
              ./go.mod
              ./go.sum
              ./vendor
              ./cmd
              ./internal
              # The test suite validates the checked-in configuration, since
              # that is what the scheduled workflow runs against.
              ./products.json
            ];
          };

          # Dependencies are vendored in-tree.
          vendorHash = null;

          nativeBuildInputs = [ pkgs.makeWrapper ];

          # Signature verification shells out to gpg.
          postInstall = ''
            wrapProgram "$out/bin/hcnix" --prefix PATH : ${lib.makeBinPath [ pkgs.gnupg ]}
          '';

          meta = {
            description = "Updater for the nix-hashicorp-tools release manifests";
            mainProgram = "hcnix";
            license = lib.licenses.mpl20;
          };
        }
      ) pkgsFor;
    in
    {
      # The overlay: adds `pkgs.hashicorp.<product>.<version>`.
      overlays.default = final: _prev: {
        hashicorp = import ./lib/packages.nix {
          pkgs = final;
          inherit (final) lib;
          system = final.stdenv.hostPlatform.system;
        };
      };

      # The complete nested tree. `packages` may only contain derivations, so
      # per-version attributes live here:
      #   nix build '.#legacyPackages.aarch64-darwin.terraform."1.9.8"'
      legacyPackages = treeFor;

      # Flat set of the latest release of each product, plus the updater.
      packages = lib.mapAttrs (
        system: tree:
        lib.genAttrs (productsFor system) (product: tree.${product}.latest)
        // {
          hcnix = hcnixFor.${system};
          default = hcnixFor.${system};
        }
      ) treeFor;

      apps = lib.mapAttrs (
        system: tree:
        lib.genAttrs (productsFor system) (product: {
          type = "app";
          program = lib.getExe tree.${product}.latest;
        })
        // {
          hcnix = {
            type = "app";
            program = lib.getExe hcnixFor.${system};
          };
          default = {
            type = "app";
            program = lib.getExe hcnixFor.${system};
          };
        }
      ) treeFor;

      checks = lib.mapAttrs (
        system: pkgs:
        let
          tree = treeFor.${system};

          # Force the derivation of every release available on this system.
          # This catches a corrupt or inconsistent manifest across the whole
          # tree, and downloads nothing: unsafeDiscardStringContext keeps the
          # drvPath as a plain string, so the check depends on each derivation
          # being *instantiated* rather than realised. Without it, `nix flake
          # check` fetches all two thousand archives.
          allDrvs = lib.concatMap (
            product:
            map (
              version: builtins.unsafeDiscardStringContext tree.${product}.${version}.drvPath
            ) tree.${product}.versions
          ) (lib.attrNames tree);
        in
        {
          eval-all = pkgs.writeText "hashicorp-eval-all" (lib.concatStringsSep "\n" allDrvs);
          hcnix = hcnixFor.${system};
        }
        # Actually fetching an archive is the only thing that proves a
        # recorded hash is right; a handful of representative products keeps
        # `nix flake check` from downloading half a gigabyte.
        // lib.genAttrs (lib.filter (product: tree ? ${product}) [
          "terraform"
          "vault"
          "consul"
          "terraform-ls"
        ]) (product: tree.${product}.latest)
      ) pkgsFor;

      devShells = lib.mapAttrs (system: pkgs: {
        default = pkgs.mkShell {
          packages = [
            hcnixFor.${system}
          ]
          ++ (with pkgs; [
            go
            gotools
            golangci-lint
            gnupg
            nixfmt
            jq
          ]);
        };
      }) pkgsFor;

      formatter = lib.mapAttrs (_: pkgs: pkgs.nixfmt-rfc-style) pkgsFor;

      templates.default = {
        path = ./templates/default;
        description = "A devShell pinned to specific HashiCorp tool versions.";
      };
    };
}
