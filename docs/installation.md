# Installation and module access

rootfs-diff is licensed under the [Apache License, Version 2.0](../LICENSE) (`Apache-2.0`). Third-party components retain their own licenses and notices.

Linux and macOS are supported hosts. These instructions target [v0.1.1](https://github.com/Azhovan/rootfs-diff/releases/tag/v0.1.1); its versioned downloads and Go commands below require that release to be published. Choose the matching amd64/arm64 prebuilt binary and verify its archive against `SHA256SUMS` using the release instructions. Using a prebuilt binary needs no Go installation.

For source builds and Go library consumers, Go **1.27.4** is pinned in `.go-version` and the `go.mod` toolchain directive; the minimum module version is **1.27.0**. Install the pinned toolchain from [go.dev/dl](https://go.dev/dl/) before offline use. An existing Go installation can download it once with:

```sh
GOTOOLCHAIN=go1.27.4 go version
```

The binary's `GOOS` and `GOARCH` describe the host running rootfs-diff. They do not need to match the CPU architecture inside the rootfs. Runtime code uses the standard library. Ordinary verification needs no hardware, root access, mounts, Docker or host tar executable.

## Clone and build

The upstream repository is public. Clone it without GitHub authentication:

```sh
git clone https://github.com/Azhovan/rootfs-diff.git
cd rootfs-diff
./scripts/verify.sh
```

Build and inspect the CLI from the repository root:

```sh
go build -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff --version
./bin/rootfs-diff --help
```

The `bin/` directory is an ignored local output location. See [CLI reference](cli.md) for usage and exit codes. v0.1.1 release packaging sets the version to `rootfs-diff v0.1.1`; ordinary source builds print `rootfs-diff dev`.

## Use the Go module

After v0.1.1 is published, download the public dependency from an existing consumer module. No GitHub credentials or `GOPRIVATE` setting are required for upstream:

```sh
go get github.com/Azhovan/rootfs-diff@v0.1.1
```

The release tag selects the v0.1.1 API. Use an audited commit or the moving `@main` branch for later development changes; Go records the resolved version in the consumer's `go.mod`. Import the root package as:

```go
import rootfsdiff "github.com/Azhovan/rootfs-diff"
```

The CLI can also be installed directly through Go:

```sh
go install github.com/Azhovan/rootfs-diff/cmd/rootfs-diff@v0.1.1
```

Go installs to `GOBIN` when set, otherwise to the applicable `GOPATH/bin`. Put that directory on your shell's `PATH` if invoking `rootfs-diff` by name. This source installation prints version `dev`; the packaged release binaries carry `v0.1.1`.

## Private copies and forks

A private copy or fork requires Git credentials with read access to that repository and private-module routing for its actual module path. Use the module path declared by that copy's `go.mod`, with the matching Git remote/import setup; creating or renaming a fork is outside these installation instructions. This section is optional for the public upstream repository.

One HTTPS credential setup is:

```sh
gh auth login
gh auth setup-git
```

An existing Git credential-helper or appropriately configured SSH transport also works. Keep credentials in that setup, never in URLs, command arguments, Go source or policies. `GOPRIVATE` controls module routing and does not grant repository access.

Before a private module's initial lookup, append its actual path to the effective `GOPRIVATE`. The following example uses `github.com/your-team/rootfs-diff`; replace it with your private module's path:

```sh
private_module=github.com/your-team/rootfs-diff
private_patterns=$(go env GOPRIVATE)
export GOPRIVATE="${private_patterns:+${private_patterns},}${private_module}"
```

Reading `go env GOPRIVATE` preserves patterns saved with `go env -w` as well as shell configuration. This export applies to the current shell and its child processes. By default, Go uses `GOPRIVATE` for both `GONOPROXY` and `GONOSUMDB`, avoiding the public proxy and checksum database for matching modules. If those variables are independently set, include the private path there too or follow your organization's private proxy setup. See the [Go private-module reference](https://go.dev/ref/mod#private-modules).

## Offline checks

After installing Go 1.27.4 and cloning the repository, the fast checks can run without downloads:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local ./scripts/verify.sh
```

This requires the selected local toolchain to satisfy `go.mod`. The [pinned Buildroot demonstration](../examples/buildroot/README.md) has separate Linux/build/download prerequisites and is not part of the offline fast check. The [synthetic example](../examples/synthetic/README.md) provides a small reproducible comparison without building firmware. See [contributor setup](../CONTRIBUTING.md) and [private-access troubleshooting](troubleshooting.md#private-access).
