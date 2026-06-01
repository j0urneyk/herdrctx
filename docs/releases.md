# Release automation

Releases are produced by GitHub Actions and GoReleaser.

The workflow is tag-driven:

```sh
git tag -a v0.0.1 -m "v0.0.1"
git push origin v0.0.1
```

When a `v*` tag is pushed, `.github/workflows/release.yml` first runs validation with read-only permissions, then runs GoReleaser with `release --clean`. GoReleaser builds archives, writes checksums, creates the GitHub Release for the tag, and uploads the artifacts.

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
