# Installation and module access

rootfs-diff is licensed under the [Apache License, Version 2.0](../LICENSE) (`Apache-2.0`). Third-party components retain their own licenses and notices.

You can run rootfs-diff on Linux or macOS, on amd64 or arm64. Download a prebuilt binary from the [v0.1.1 release](https://github.com/Azhovan/rootfs-diff/releases/tag/v0.1.1). Choose the binary for your operating system and CPU architecture, then verify the downloaded archive against `SHA256SUMS` as described in the release instructions. You do not need Go to use a prebuilt binary.

For source builds and library development, use Go **1.27.4**, the version recorded in `.go-version` and the `go.mod` toolchain directive. The module requires at least Go **1.27.0**. Install Go 1.27.4 from [go.dev/dl](https://go.dev/dl/) before working offline, or use an existing Go installation to download it:

```sh
GOTOOLCHAIN=go1.27.4 go version
```

`GOOS` and `GOARCH` identify the operating system and CPU of the computer running rootfs-diff. They do not need to match the firmware's CPU architecture, because rootfs-diff reads archive contents without executing them. The application uses only the Go standard library. Routine verification needs no hardware, root access, mounts, Docker or system `tar` command.

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

Git ignores the `bin/` directory, so the built command stays local. See the [CLI reference](cli.md) for usage and exit codes. Packaged v0.1.1 binaries report `rootfs-diff v0.1.1`; a normal source build reports `rootfs-diff dev`.

## Use the Go module

Run this command from your own Go module to add the dependency. The public upstream module requires neither GitHub credentials nor a `GOPRIVATE` setting:

```sh
go get github.com/Azhovan/rootfs-diff@v0.1.1
```

The release tag selects the v0.1.1 API. To use later development changes, choose a commit you have reviewed or use `@main`, which follows the latest development branch. Go records the resolved version in your module's `go.mod`. Import the root package as:

```go
import rootfsdiff "github.com/Azhovan/rootfs-diff"
```

The CLI can also be installed directly through Go:

```sh
go install github.com/Azhovan/rootfs-diff/cmd/rootfs-diff@v0.1.1
```

Go installs the command in `GOBIN` if that variable is set, or in `GOPATH/bin` otherwise. Add that directory to your shell's `PATH` to run `rootfs-diff` by name. This source installation reports version `dev`; packaged release binaries report `v0.1.1`.

## Private copies and forks

To use a private copy or fork, configure Git credentials with read access to that repository. You must also tell Go which module paths are private. Use the module path declared in the private copy's `go.mod`, and make sure its Git remote and import paths agree. These instructions assume that copy is already set up. You can skip this section when using the public upstream repository.

One HTTPS credential setup is:

```sh
gh auth login
gh auth setup-git
```

You can also use an existing Git credential helper or a configured SSH transport. Store credentials in that setup, never in URLs, command arguments, Go source or policies. `GOPRIVATE` tells Go how to fetch private modules; it does not grant access to the repository.

Before fetching a private module for the first time, add its path to the current `GOPRIVATE` value. Replace `github.com/your-team/rootfs-diff` in this example with your private module's path:

```sh
private_module=github.com/your-team/rootfs-diff
private_patterns=$(go env GOPRIVATE)
export GOPRIVATE="${private_patterns:+${private_patterns},}${private_module}"
```

The example reads `go env GOPRIVATE` first so it preserves patterns saved with `go env -w` or set in the shell. The export applies to the current shell and its child processes. By default, Go also uses `GOPRIVATE` for `GONOPROXY` and `GONOSUMDB`, so matching modules bypass the public module proxy and checksum database. If you have set either variable separately, add the private path to those settings too, or follow your organization's private proxy configuration. See the [Go private-module reference](https://go.dev/ref/mod#private-modules).

## Offline checks

After installing Go 1.27.4 and cloning the repository, run the routine checks with downloads disabled:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local ./scripts/verify.sh
```

The selected local toolchain must satisfy `go.mod`. The [Buildroot demonstration](../examples/buildroot/README.md) has separate Linux build requirements and downloads, so it is not part of this offline check. Try the [synthetic example](../examples/synthetic/README.md) for a small comparison that does not require a firmware build. See also [contributor setup](../CONTRIBUTING.md) and [private-access troubleshooting](troubleshooting.md#private-access).
