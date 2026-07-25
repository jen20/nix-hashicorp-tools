# Entry point for consumers that are not using flakes.
#
#   let hashicorp = import (fetchTarball "https://github.com/jen20/nix-hashicorp-tools/archive/main.tar.gz") {};
#   in hashicorp.terraform."1.9.8"
{
  nixpkgs ? <nixpkgs>,
  system ? builtins.currentSystem,
  pkgs ? import nixpkgs {
    inherit system;
    # Most of these tools are BUSL licensed, which nixpkgs treats as unfree.
    config.allowUnfree = true;
  },
}:

import ./lib/packages.nix {
  inherit pkgs system;
  inherit (pkgs) lib;
}
