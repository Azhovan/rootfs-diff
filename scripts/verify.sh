#!/bin/sh
set -eu

cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

# Check formatting without changing source files. go list limits the check to
# package sources, excluding ignored local generated artifacts.
files=$(go list -f '{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .TestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' ./...)
unformatted=$(printf '%s\n' "$files" | while IFS= read -r file; do
    if [ -n "$file" ]; then gofmt -l "$file"; fi
done)
if [ -n "$unformatted" ]; then
    printf 'Go files need formatting:\n%s\n' "$unformatted" >&2
    exit 1
fi

go vet ./...
go test ./...

# Keep the build outside the checkout and remove it even on interruption.
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/rootfs-diff-verify.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
go build -o "$build_dir/rootfs-diff" ./cmd/rootfs-diff
go run ./examples/library > "$build_dir/library-report.json"
