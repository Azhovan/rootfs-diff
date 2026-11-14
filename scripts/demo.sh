#!/bin/sh
set -eu

cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

# The pinned Go toolchain must already be installed. The demonstration needs no
# network, container, root access, system tar, mounting, or target execution.
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
demo_dir=$(mktemp -d "${TMPDIR:-/tmp}/rootfs-diff-demo.XXXXXX")
trap 'rm -rf "$demo_dir"' EXIT HUP INT TERM
go build -o "$demo_dir/rootfs-diff" ./cmd/rootfs-diff
go build -o "$demo_dir/demo" ./examples/synthetic/cmd/demo
"$demo_dir/demo" -cli "$demo_dir/rootfs-diff" -out "$demo_dir/inputs"
