# Release automation

Releases are produced by GitHub Actions and GoReleaser.

The workflow is tag-driven:

```sh
git tag -a v0.0.1 -m "v0.0.1"
git push origin v0.0.1
```

When a `v*` tag is pushed, `.github/workflows/release.yml` calls the same `ci.yml` workflow used for pull requests and pushes to `main`. The local workflow reference validates the tagged commit with read-only permissions.

Validation runs tests, `go vet`, a build with `CGO_ENABLED=0`, and binary version/help checks on Ubuntu 24.04 and macOS 15, each on x86_64 and arm64. Formatting and lint run once on Linux x86_64. Every validation job must succeed before publishing; a failure or cancellation blocks the release job. See the [testing guide](testing.md) for the runner matrix and validation scope.

After validation, GoReleaser runs with `release --clean` to build archives, write checksums, create the GitHub Release, and upload artifacts. The release job then updates the source-built Homebrew formula in [`j0urneyk/homebrew-tap`](https://github.com/j0urneyk/homebrew-tap).

## Target matrix

`herdrctx` follows the same OS and architecture families Herdr supports:

| OS | Architecture | GoReleaser target |
| --- | --- | --- |
| Linux | x86_64 | `linux_amd64` |
| Linux | aarch64 | `linux_arm64` |
| macOS | x86_64 | `darwin_amd64` |
| macOS | aarch64 | `darwin_arm64` |

Native Windows binaries are not published because Herdr does not currently support native Windows. Windows users should use the Linux build under WSL.

## Required GitHub permissions

The workflow defaults to read-only permissions. Only the publish job sets:

```yaml
permissions:
  contents: write
```

This lets GoReleaser create GitHub Releases and upload assets with the default `GITHUB_TOKEN` after validation passes.

The release job also needs the `HOMEBREW_TAP_GITHUB_TOKEN` repository secret to push formula updates to `j0urneyk/homebrew-tap`. Use a token with write access to the tap repository.

## Local snapshot builds

Use a snapshot build before pushing a tag:

```sh
make snapshot
```

This runs:

```sh
goreleaser release --snapshot --clean
```

Snapshot artifacts are written to `dist/` and are not published.
