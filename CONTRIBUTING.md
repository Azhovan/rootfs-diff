# Contributor setup

Linux and macOS are supported hosts. The repository pins Go **1.27.4**.
The module minimum is Go **1.27.0**. `.go-version` pins the exact
development version and `go.mod` requests `toolchain go1.27.4`.

Install Go 1.27.4 from [go.dev/dl](https://go.dev/dl/), or let an existing Go
installation download it once with `GOTOOLCHAIN=go1.27.4 go version`. Once the
toolchain is installed, the checks below need no network. Runtime code and tests
use only the standard library. No hardware, root access, Docker, mounting or host
tar command is needed for routine tests. The [Buildroot demonstration](examples/buildroot/README.md)
has separate Linux/build/download prerequisites; the [synthetic example](examples/synthetic/README.md)
provides a small reproducible comparison without a Buildroot build.

Clone this public repository without authentication:

```sh
git clone https://github.com/Azhovan/rootfs-diff.git
cd rootfs-diff
go version
./scripts/verify.sh
```

Library consumers can fetch the public module normally from their own Go module:

```sh
go get github.com/Azhovan/rootfs-diff@main
```

The upstream module needs no `GOPRIVATE` setting. For private copies or forks,
see [private-module setup](docs/installation.md#private-copies-and-forks), including
preserving existing effective patterns. Keep credentials out of URLs and source.

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
The verification script checks formatting without modifying files, runs vet and
tests (including public examples and executable subprocess tests), builds the CLI
and runs the complete library example. See [CI and full checks](docs/ci.md#repository-verification)
for race, bounded fuzz and four cross-build commands.

`bin/`, `tmp/`, `artifacts/`, `coverage.out` and `*.test` are ignored locations for
local outputs. Do not commit generated images, toolchains or build trees.

Shared model, module and verification changes need coordination with their
component consumers. Tests live with each component. Author expected manifests
and reports independently of production scanner/comparison code; the
[archive fixture helper](internal/testfixture/README.md) writes small deterministic
inputs using Go's standard archive writers.
