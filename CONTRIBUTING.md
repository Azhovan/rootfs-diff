# Contributor setup

Linux and macOS are supported hosts. Foundation pins Go **1.27.4**, the current
stable version listed by the [official download feed](https://go.dev/dl/?mode=json)
when implemented. The module minimum is Go **1.27.0**. `.go-version` pins the exact
development version and `go.mod` requests `toolchain go1.27.4`.

Install Go 1.27.4 from [go.dev/dl](https://go.dev/dl/), or let an existing Go
installation download it once with `GOTOOLCHAIN=go1.27.4 go version`. Once the
toolchain is installed, the checks below need no network. Runtime code and tests
use only the standard library. No hardware, root access, Docker, mounting or host
tar command is needed for routine tests. The later Buildroot proof has separate
Linux and download prerequisites.

Authenticate GitHub for this private repository before cloning:

```sh
gh auth login
gh auth setup-git
git clone https://github.com/Azhovan/rootfs-diff.git
cd rootfs-diff
go version
./scripts/verify.sh
```

Library consumers need Git credentials with read access to this repository.
Configure `GOPRIVATE=github.com/Azhovan/rootfs-diff` in the caller's environment
before downloading the module, so Go bypasses the public module proxy and checksum
database for this private path. Preserve other existing `GOPRIVATE` patterns. Do
not embed tokens in URLs, source files or reports.

```sh
GOPRIVATE=github.com/Azhovan/rootfs-diff go get github.com/Azhovan/rootfs-diff@main
```

Useful commands from the repository root:

```sh
./scripts/verify.sh       # formatting check, vet, tests and executable examples
go test ./...
go vet ./...
go test -race ./...
go doc .
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local ./scripts/verify.sh
```

The last command requires Go 1.27.0 or newer already installed; use the pinned
1.27.4 for reproducible development. It proves checks do not fetch dependencies.
The verification script checks formatting without modifying files. The CLI owner
will add a real command build and subprocess checks when the CLI is delivered.
Parser fuzz targets arrive with their parsers, not as placeholders in foundation.

`bin/`, `tmp/`, `artifacts/`, `coverage.out` and `*.test` are ignored locations for
local outputs. Do not commit generated images, toolchains or build trees.

Shared model, module and verification changes need coordination with their
component consumers. Tests live with each component. Author expected manifests
and reports independently of production scanner/comparison code; the
[archive fixture helper](internal/testfixture/README.md) writes small deterministic
inputs using Go's standard archive writers.
