# Security

Report a vulnerability through a private GitHub Security Advisory at <https://github.com/openbunny/wormswmd/security/advisories/new>. Do not open a public issue for it.

Include the command, the error text, and the version line from `wormswmd version`. Omit save files, config bodies, and game binaries.

## Supported versions

The latest commit on `main` is the only supported version. This file makes no response-time commitment.

## What the tool writes

[README.md](README.md#what-it-changes) lists every change that `fix` and `apply` make to the game app, and [Undo](README.md#undo) describes the restore commands.

## Release artifacts

Each release carries `wormswmd_VERSION_darwin_ARCH.tar.gz` archives, a CycloneDX SBOM for each archive, `checksums.txt` and `checksums.txt.bundle`. The release workflow `.github/workflows/release.yml` signs `checksums.txt` with keyless [cosign](https://docs.sigstore.dev/cosign/signing/overview/): the signing certificate is issued to the workflow identity through GitHub Actions OIDC and recorded in the Sigstore transparency log. No long-lived signing key exists. The bundle holds the signature, the certificate and the log entry.

Verify the signature, then the archive:

```console
cosign verify-blob --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/openbunny/wormswmd/.github/workflows/release.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
shasum -a 256 --check --ignore-missing checksums.txt
```

`cosign verify-blob` fails when the signature, the certificate identity or the issuer does not match. The identity pattern limits accepted signatures to this workflow on a `v*` tag. A passing check shows that the workflow built the files; it does not show that the source is free of defects.

The binaries are not notarized by Apple and carry no Developer ID signature. Gatekeeper blocks them until `xattr -d com.apple.quarantine` runs on the binary; the Homebrew cask runs it after install. Remove the attribute only after the signature check passes.

The Homebrew cask in `OA/homebrew-tap` pins the SHA-256 of the archive. Renovate takes that value from the digest GitHub computes for the release asset.

## Trust in the Qt archive

The Qt archive was assembled by the author of the upstream WormsWMD-macOS-Fix from Homebrew bottles; `SOURCE_PROVENANCE.tsv` inside it lists every input with its checksum. This project mirrors it unchanged in its `v0.1.0` release and does not rebuild it. The default, `WORMSWMD_QT` and `--qt` paths check the archive against the SHA-256 compiled into `wormswmd` before extraction. Two inputs bypass that check: `--qt-prefix` copies an extracted Qt directory without any checksum, and `--qt-sha256` replaces the compiled-in checksum (and is accepted only with `--qt` or `WORMSWMD_QT`). Both print a warning. Code from these inputs runs inside the game and the user is responsible for it.
