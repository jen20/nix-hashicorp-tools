{
  description = "A development environment pinned to specific HashiCorp tool versions.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    flake-utils.url = "github:numtide/flake-utils";

    hashicorp-tools = {
      url = "github:jen20/nix-hashicorp-tools";
      # The overlay builds against whichever nixpkgs it is applied to, so this
      # changes nothing about what is built; it just keeps a second, unused
      # nixpkgs out of this flake's lock file.
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      hashicorp-tools,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ hashicorp-tools.overlays.default ];
          # Terraform, Vault, Consul, Nomad, Packer and Boundary have been
          # BUSL licensed since 2023, which nixpkgs treats as unfree.
          config.allowUnfree = true;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            # Pin an exact release...
            pkgs.hashicorp.terraform."1.9.8"
            # ...or track the newest one.
            pkgs.hashicorp.vault.latest
          ];
        };
      }
    );
}
