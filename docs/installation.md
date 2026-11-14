# Installation and module access

Linux and macOS are supported hosts. Go **1.27.4** is pinned in `.go-version` and the `go.mod` toolchain directive; the minimum module version is **1.27.0**. Install the pinned toolchain from [go.dev/dl](https://go.dev/dl/) before offline use. An existing Go installation can download it once with:

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

No release binaries are published by this work. The `bin/` directory is an ignored local output location. See [CLI reference](cli.md) for usage and exit codes.

## Use the Go module

In an existing consumer module, download the public dependency normally. No GitHub credentials or `GOPRIVATE` setting are required for upstream:

```sh
go get github.com/Azhovan/rootfs-diff@main
```

`main` moves. For reproducible consumers, select an audited commit instead; Go records its resolved pseudo-version in the consumer's `go.mod`. Import the root package as:

```go
import rootfsdiff "github.com/Azhovan/rootfs-diff"
```

The CLI can also be installed directly through Go:

```sh
go install github.com/Azhovan/rootfs-diff/cmd/rootfs-diff@main
```

Go installs to `GOBIN` when set, otherwise to the applicable `GOPATH/bin`. Put that directory on your shell's `PATH` if invoking `rootfs-diff` by name.

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

This requires the selected local toolchain to satisfy `go.mod`. The separately owned Buildroot demonstration has Linux/build/download prerequisites and is not part of the offline fast check; see [issue #8](https://github.com/Azhovan/rootfs-diff/issues/8) for recipe/results status. See [contributor setup](../CONTRIBUTING.md) and [private-access troubleshooting](troubleshooting.md#private-access).
