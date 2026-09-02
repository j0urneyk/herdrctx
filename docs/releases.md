# Release automation

Releases are produced by GitHub Actions and GoReleaser.

The workflow is tag-driven:

```sh
git tag -a v0.0.1 -m "v0.0.1"
git push origin v0.0.1
```

When a `v*` tag is pushed, `.github/workflows/release.yml` calls the same `ci.yml` workflow used for pull requests and pushes to `main`. The local workflow reference validates the tagged commit with read-only permissions.

Validation runs tests, `go vet`, a build with `CGO_ENABLED=0`, binary version/help checks, and a real Herdr 0.6.5 session lifecycle test on Ubuntu 24.04 and macOS 15, each on x86_64 and arm64. Formatting and lint run once on Linux x86_64. Every validation job must succeed before publishing; a failure or cancellation, including an integration test failure, blocks the release job. See the [testing guide](testing.md) for the runner matrix and validation scope.

After validation, GoReleaser runs with `release --clean` to build archives, write checksums, create the GitHub Release, and upload artifacts. The release job then updates the source-built Homebrew formula in [`j0urneyk/homebrew-tap`](https://github.com/j0urneyk/homebrew-tap).

## Plugin versions and publication

The root `herdr-plugin.toml` pins the binary version installed by `scripts/install.sh`. The first manifest uses the published `v0.0.2` assets. Herdr's plugin system started in [0.7.0](https://github.com/herdrdev/herdr/releases/tag/v0.7.0), which supports build commands and offline GitHub installation. The standalone CLI's minimum remains 0.6.5. On 0.7.0, local `plugin link` needs a running server; GitHub installation has its own offline registration path.

For a new release:

1. Update the manifest's `version` alongside the release changes. Prepare and review the changes and validation results before committing, pushing, or publishing.
2. Tag that commit as `v<manifest-version>` after approval. The release workflow checks this exact match before GoReleaser publishes anything. Normal CI uses local installer fixtures and never requires the new release assets to exist.
3. Verify all four archives and `checksums.txt` in the published release, then run the isolated [public plugin installation check](testing.md#plugin-installation).
4. Confirm the default branch contains the verified manifest and installer. After approval, add the `herdr-plugin` repository topic. The repository must be public, non-fork, and not archived.
5. Check the repository, root manifest path, and version in the [official index](https://assets.herdr.dev/plugins/index.json) and [Marketplace](https://herdr.dev/plugins/) after its 30-minute refresh. See the [listing requirements](https://herdr.dev/docs/marketplace/).

Installation from `main` can fail between merging a version bump and publishing its assets. The installer reports the missing asset and preserves an existing binary; it never substitutes another version. During this window, select the previous published plugin revision with `herdr plugin install j0urneyk/herdrctx --ref <previous-plugin-tag-or-commit>`. Only revisions containing both plugin files work. The original `v0.0.2` tag predates those files; the initial plugin revision below can be used as a fallback.

The initial plugin revision is [`b2c0833`](https://github.com/j0urneyk/herdrctx/commit/b2c0833d19b6be2ffd137304ca3f145284d81785), which installs binary version 0.0.2:

```sh
herdr plugin install j0urneyk/herdrctx --ref b2c0833d19b6be2ffd137304ca3f145284d81785
```

The manifest version describes the installed binary, so an installer-only change may keep that version. Any new binary release tag must match the manifest. Herdr uninstallation removes its managed checkout, while the binary installed in `HERDRCTX_INSTALL_DIR` (default `~/.local/bin`) remains until the user removes it.

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
