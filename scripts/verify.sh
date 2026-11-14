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

# The CLI owner adds the real command build and subprocess tests when landed.
