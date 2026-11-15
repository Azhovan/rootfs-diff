# Contributor setup

You can develop and run rootfs-diff on Linux or macOS. Use Go **1.27.4**
for reproducible development builds. This version is recorded in `.go-version`
and requested by the `toolchain go1.27.4` directive in `go.mod`. The module's
minimum Go version is **1.27.0**.

Install Go 1.27.4 from [go.dev/dl](https://go.dev/dl/), or let an existing Go
installation download it with `GOTOOLCHAIN=go1.27.4 go version`. After this
one-time setup, you can run the checks below without a network connection.
The application and its tests use only the Go standard library. Routine tests
need no hardware, root access, Docker, mounts or system `tar` command.
The [Buildroot demonstration](examples/buildroot/README.md) has separate Linux
build requirements and downloads. Try the [synthetic example](examples/synthetic/README.md)
for a small comparison that does not require a Buildroot build.

Clone this public repository without authentication:

```sh
git clone https://github.com/Azhovan/rootfs-diff.git
cd rootfs-diff
go version
./scripts/verify.sh
```

To use the public library from your own Go module, run:

```sh
go get github.com/Azhovan/rootfs-diff@main
```

You do not need to set `GOPRIVATE` for the upstream module. For a private copy
or fork, follow the [private-module setup](docs/installation.md#private-copies-and-forks)
and preserve any existing `GOPRIVATE` patterns. Keep credentials out of URLs
and source files.

Useful commands from the repository root:

```sh
./scripts/verify.sh       # formatting check, vet, tests and executable examples
go test ./...
go vet ./...
go test -race ./...
go doc .
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local ./scripts/verify.sh
```

The last command requires Go 1.27.0 or newer to be installed locally; use
1.27.4 for reproducible development. It disables dependency downloads during
verification.
The verification script checks formatting without modifying files, runs vet and
tests (including public examples and executable subprocess tests), builds the CLI
and runs the complete library example. See [CI and full checks](docs/ci.md#repository-verification)
for race, bounded fuzz and four cross-build commands.

`bin/`, `tmp/`, `artifacts/`, `coverage.out` and `*.test` are ignored locations for
local outputs. Keep generated images, toolchains and build trees out of commits.

Coordinate changes to shared models, the module and verification scripts with
the people working on the components that use them. Keep tests with the
component they cover. Write expected manifests and reports independently of
the scanner and comparison code being tested. The
[archive fixture helper](internal/testfixture/README.md) creates small,
deterministic inputs using Go's standard archive writers.
