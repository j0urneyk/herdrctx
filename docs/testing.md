# Testing

Use Go 1.26.3 from `go.mod`. CI uses golangci-lint 2.12.0 for formatting and lint.

```sh
go test ./...
go vet ./...
golangci-lint run
go build -o bin/herdrctx ./cmd/herdrctx
```

The unit tests use a fake Herdr command and do not require Herdr to be installed.

## Native CI checks

The shared `.github/workflows/ci.yml` workflow runs on pull requests, pushes to `main`, and calls from the release workflow.

| Platform | Runner | Go target |
| --- | --- | --- |
| Linux x86_64 | `ubuntu-24.04` | `linux/amd64` |
| Linux arm64 | `ubuntu-24.04-arm` | `linux/arm64` |
| macOS x86_64 | `macos-15-intel` | `darwin/amd64` |
| macOS arm64 | `macos-15` | `darwin/arm64` |

Each job checks the Go host and target architecture, runs tests and vet, and builds with `CGO_ENABLED=0` to match the release configuration. It then executes `--version` and `--help`, checking both exit status and output. A failure on one platform leaves the other jobs running. A separate Linux x86_64 job checks formatting and lint.

These checks cover the listed OS versions. They do not establish compatibility with every Linux distribution, older macOS releases, or every terminal application. Real Herdr session creation, terminal handoff, detach, and cleanup still need a separate integration test.

## Results

Open the [CI runs](https://github.com/j0urneyk/herdrctx/actions/workflows/ci.yml) and select the run for the commit you are checking. Confirm all four platform jobs and the format/lint job succeeded, and use each job's step logs to investigate failures. Local results alone do not establish that the remote matrix passed.
