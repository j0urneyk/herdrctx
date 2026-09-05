# Releases

Use this guide to prepare a release, publish it through GitHub Actions, and verify the result. It also explains how plugin installation depends on published assets. Follow [AGENTS.md](../AGENTS.md) for Git authorization and workspace safety: preparing files and checking a release does not itself authorize committing, pushing, or publishing.

The release path is: prepare the release commit, validate locally, push its version tag, wait for shared CI, publish with GoReleaser, then update Homebrew. The [release workflow](../.github/workflows/release.yml) implements the remote steps; the [testing guide](testing.md#choosing-checks) defines local checks and CI coverage.

## Prepare the release

Confirm the intended release changes and current worktree state before editing. Preserve unrelated work. Keep the project, command, module, manifest, and artifacts named `herdrctx`.

1. Set `version` in [herdr-plugin.toml](../herdr-plugin.toml) to the intended binary version. The release tag must be exactly `v<manifest-version>`. Installer-only changes may keep the binary version; a new binary release may not use a mismatched tag.
2. Check user-visible behavior against both READMEs and [UI behavior](ui.md). Preserve release-note context and contributor credit in the relevant PR or commit text. Keep Go aligned between [go.mod](../go.mod) and [.tool-versions](../.tool-versions), and keep packaging within the [target matrix](#target-matrix).
3. Run the applicable [local checks](testing.md#choosing-checks) and a [snapshot build](#local-snapshot-builds). Review the archives, version stamping, and checksums. Local fixtures do not require the new release assets to exist.
4. Confirm publishing authorization, the exact commit to tag, and the [GitHub permissions and secret](#required-github-permissions). Finish the authorized commit/push work before tagging. Do not tag a checkout containing uncommitted release changes: Git tags identify commits, not working-tree contents.

[.goreleaser.yaml](../.goreleaser.yaml) generates the changelog in ascending order and excludes entries beginning with `docs:`, `test:`, or `chore:`. There is currently no tracked `CHANGELOG.md` or `Unreleased` section. Review the release commits and their expected changelog entries before tagging, then verify the generated release body at closeout. The workflow publishes automatically after validation; it has no separate release-note approval step.

## Local snapshot builds

Use Go from `go.mod` and GoReleaser matching the release workflow (currently `v2.16.0`). From the repository root:

```sh
make snapshot
```

The [Makefile](../Makefile) runs `goreleaser release --snapshot --clean`. [Snapshot mode](https://goreleaser.com/customization/publish/snapshots/) writes local artifacts under `dist/` without publishing. **The `--clean` option removes the previous `dist/`, including integration logs**, so preserve evidence you still need before running it.

Review the four archives and `checksums.txt`, inspect archive contents for the `herdrctx` executable, and run the native binary's `--version` and `--help`. Snapshot versions are not final release versions. A snapshot confirms local packaging; it does not prove remote CI, final tag/manifest alignment, GitHub installation, or the Homebrew update.

## Publish the tag

Only after the release commit is ready and publication is authorized, derive the tag from its manifest and inspect that version before executing the tag/push commands:

```sh
release_version=$(awk -F '"' '/^version = / { print $2; exit }' herdr-plugin.toml)
git tag -a "v$release_version" -m "v$release_version"
git push origin "v$release_version"
```

Pushing a `v*` tag triggers [release.yml](../.github/workflows/release.yml). It calls the local [ci.yml](../.github/workflows/ci.yml) at the tagged commit with read-only permissions. All four native jobs and the format/lint job must succeed before the publish job starts. This includes installer fixtures, real Herdr lifecycle tests, and local plugin registration/nested guards; see [Native CI checks](testing.md#native-ci-checks).

The publish job fetches full Git history and checks the tag against the manifest before GoReleaser runs. GoReleaser executes `release --clean`, builds archives and checksums, and publishes a GitHub Release with assets. Releases are not drafts; prerelease detection is automatic.

The final workflow step updates `Formula/herdrctx.rb` in [`j0urneyk/homebrew-tap`](https://github.com/j0urneyk/homebrew-tap). The formula builds from the tagged source archive, verifies its SHA-256, and injects the version into the Go build. This step runs after GitHub publication, so its failure can leave a published release with a stale Homebrew formula.

## Target matrix

Publish only these four targets. The exact archive names come from [.goreleaser.yaml](../.goreleaser.yaml) and are also expected by [scripts/install.sh](../scripts/install.sh). Here `<version>` is the binary version without the tag's leading `v`.

| Platform | GoReleaser target | Archive |
| --- | --- | --- |
| macOS x86_64 | `darwin_amd64` | `herdrctx_<version>_macos_x86_64.tar.gz` |
| macOS arm64 | `darwin_arm64` | `herdrctx_<version>_macos_aarch64.tar.gz` |
| Linux x86_64 | `linux_amd64` | `herdrctx_<version>_linux_x86_64.tar.gz` |
| Linux arm64 | `linux_arm64` | `herdrctx_<version>_linux_aarch64.tar.gz` |

All builds use `CGO_ENABLED=0`, `./cmd/herdrctx`, and `-s -w -X main.version={{ .Version }}`. The checksum file is named `checksums.txt`. No native Windows artifact is configured or permitted by the current project policy; the [native CI matrix](testing.md#native-ci-checks) defines what platforms are actually tested.

## Verify the published release

Use the exact tag and its workflow run, not a previous successful run or an unqualified latest release.

1. Confirm shared validation passed and inspect both the GoReleaser and Homebrew steps. Confirm the GitHub Release points to the intended tag/commit and has the expected prerelease status.
2. Confirm all four archives above and `checksums.txt` are present. Download the archives to a scratch directory and verify each SHA-256 against its unique checksum entry. Inspect each archive for the executable. On a matching native host, verify `herdrctx --version` reports the manifest version and `--help` succeeds; archive presence alone does not prove execution on another architecture.
3. Compare the published release notes with the reviewed changes and generated changelog. Correct missing or stale release documentation before declaring the release complete.
4. Run the isolated [public GitHub installation check](testing.md#public-github-installation) with the published tag. It checks the selected manifest and installed binary together. Follow its prerequisites, including the local binary that the harness resolves at startup. For an extracted native release binary, the [lifecycle](testing.md#real-herdr-lifecycle) and [local registration](testing.md#local-registration-and-nested-guards) checks also accept `--binary`.
5. Confirm the tap formula now uses this tag's source URL and checksum and builds with the matching version. A successful formula-update step proves the update was pushed; the release workflow does not run a Homebrew installation test. Report that limit unless separately checked.

Record the tag/commit, workflow result, assets and native platforms checked, public-install result, and Homebrew state in the release handoff. Distinguish completed checks from ones that could not run. For marketplace publication, also follow the section below.

## Plugin versions and publication

The root manifest's version selects the **binary release**, while `herdr plugin install ... --ref <tag-or-commit>` selects the **installer checkout**. That checkout must contain both `herdr-plugin.toml` and `scripts/install.sh`, and the release named by its manifest must be published. The manifest requires Herdr `0.7.0`; the standalone CLI minimum remains `0.6.5`.

The plugin declares only a build command, `sh scripts/install.sh`. It installs the standalone CLI, with no Herdr pane, action, startup hook, event, or link handler. The installer downloads the selected platform archive and `checksums.txt` from `releases/download/v<manifest-version>`, verifies the archive, stages the executable in the destination directory, and replaces the installed binary. It does not substitute another version when assets are missing. See the [README installation instructions](../README.md#managing-plugin-installations) for PATH, custom destinations, reinstall, and removal behavior.

Installation from the default branch can fail between merging a manifest bump and publishing its assets. The installer reports the missing asset and preserves an existing binary. During this window, choose a previous published plugin revision with `--ref`; a binary-only tag without plugin files cannot serve as that fallback.

### Marketplace listing

Once public installation is verified, confirm that the default branch contains the intended manifest and installer. For initial discovery, add the `herdr-plugin` repository topic only when that publication action is authorized. The repository must be public, non-fork, and not archived, with parseable manifest metadata. These are [Herdr's listing requirements](https://herdr.dev/docs/marketplace/).

Check the repository, root manifest path, version, and default-branch commit in the [official index](https://assets.herdr.dev/plugins/index.json) and [Marketplace](https://herdr.dev/plugins/) after the documented 30-minute refresh. A listing is discovery metadata; the separate public-install test establishes that the selected revision's build command and downloads work.

### Initial plugin revision

The original `v0.0.2` tag predates plugin files, so it cannot be installed as a plugin revision. Commit [`b2c0833`](https://github.com/j0urneyk/herdrctx/commit/b2c0833d19b6be2ffd137304ca3f145284d81785) added the manifest and installer while pinning binary version `0.0.2`:

```sh
herdr plugin install j0urneyk/herdrctx --ref b2c0833d19b6be2ffd137304ca3f145284d81785
```

This is a historical fallback for that binary, not the normal release target. It still depends on the `v0.0.2` assets being available. For later releases, use a verified revision containing the plugin files and the intended binary version.

## Required GitHub permissions

The release workflow defaults to read-only permissions. Only the publish job has `contents: write`, allowing GoReleaser to create the release and upload assets through `GITHUB_TOKEN` after validation passes.

The publish job also needs the repository secret `HOMEBREW_TAP_GITHUB_TOKEN`, with write access to `j0urneyk/homebrew-tap`. The script checks this secret only in the Homebrew step, after GoReleaser has published. Verify that it is configured before pushing a release tag; never put its value in documentation or logs.

## Handling a failed release

If shared validation fails or is cancelled, publishing is blocked. Use the [test logs and artifacts](testing.md#results-and-failure-diagnosis) to identify the failure, fix it, and rerun the affected checks before another authorized publication attempt. A tag/manifest mismatch also stops before GoReleaser runs. Do not move an existing tag or bypass validation to hide the failure.

If GoReleaser fails, inspect the GitHub Release and its assets before retrying: publication may be partial. If Homebrew fails, check the already-published GitHub Release separately and report the stale tap. The workflow has no dedicated Homebrew-only recovery job, and rerunning the publish job repeats GoReleaser. Inspect the completed steps and existing artifacts before choosing an authorized recovery action; do not assume a whole-job rerun is safe or that a red workflow means nothing was published.

If public plugin installation fails, compare the selected checkout's manifest with the published tag, asset names, and checksums, then inspect the isolated test logs. Preserve any existing user binary and use a previously verified plugin revision when a fallback is needed. If only marketplace metadata is stale, check default-branch metadata, repository eligibility, and index refresh before changing binary assets.
