# Development shell for consumers of `nix-shell` rather than `nix develop`.
(import (
  let
    lock = (builtins.fromJSON (builtins.readFile ./flake.lock)).nodes.flake-compat.locked;
  in
  fetchTarball {
    url = "https://github.com/edolstra/flake-compat/archive/${lock.rev}.tar.gz";
    sha256 = lock.narHash;
  }
) { src = ./.; }).shellNix
