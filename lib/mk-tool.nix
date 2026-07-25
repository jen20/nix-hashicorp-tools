{
  lib,
  stdenv,
  stdenvNoCC,
  fetchurl,
  unzip,
  autoPatchelfHook,
}:

# Build a single release of a single HashiCorp product from the official
# binary archive published on releases.hashicorp.com.
{
  product,
  version,
  # Nix-side hash of the release archive. HashiCorp's SHA256SUMS files contain
  # hashes of the zip itself, which is exactly what fetchurl wants.
  sha256,
  # Go-style OS/architecture, as used in the release archive filename.
  os,
  arch,
  description ? "",
  homepage ? "",
  mainProgram ? product,
  license ? null,
  platforms ? [ ],
}:

stdenvNoCC.mkDerivation {
  pname = product;
  inherit version;

  src = fetchurl {
    url = "https://releases.hashicorp.com/${product}/${version}/${product}_${version}_${os}_${arch}.zip";
    inherit sha256;
  };

  # The archives are flat: one or more binaries plus some .txt files. Unpack
  # into a directory of our own rather than setting sourceRoot to ".", which
  # would make the build root itself the source and sweep up the files stdenv
  # leaves there.
  unpackPhase = ''
    runHook preUnpack

    mkdir -p source
    unzip -q "$src" -d source
    cd source

    runHook postUnpack
  '';

  nativeBuildInputs = [
    unzip
  ]
  ++ lib.optional stdenv.hostPlatform.isLinux autoPatchelfHook;

  dontConfigure = true;
  dontBuild = true;

  # The Linux binaries are static Go builds and the Darwin binaries carry
  # HashiCorp's code signature; stripping either gains nothing and breaks the
  # latter.
  dontStrip = true;

  # Install every regular file in the archive root that is not documentation.
  # Doing this by exclusion rather than by name means multi-binary archives
  # (tfc-agent ships tfc-agent and tfc-agent-core) work without special-casing.
  installPhase = ''
    runHook preInstall

    mkdir -p "$out/bin" "$out/share/doc/${product}"

    for f in *; do
      [ -f "$f" ] || continue
      case "$f" in
        *.txt | *.md | *.html)
          install -Dm444 "$f" "$out/share/doc/${product}/$f"
          ;;
        *)
          install -Dm555 "$f" "$out/bin/$f"
          ;;
      esac
    done

    if [ -z "$(ls -A "$out/bin")" ]; then
      echo "error: ${product} ${version} ${os}_${arch} archive contained no binaries" >&2
      exit 1
    fi

    rmdir "$out/share/doc/${product}" "$out/share/doc" "$out/share" 2>/dev/null || true

    runHook postInstall
  '';

  meta = {
    inherit
      description
      homepage
      mainProgram
      platforms
      ;
    sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
  }
  // lib.optionalAttrs (license != null) { inherit license; };
}
