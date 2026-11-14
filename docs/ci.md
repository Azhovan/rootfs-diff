# CI usage

This is an ordinary workflow step, not a custom GitHub Action. Prepare the source/toolchain, build `./bin/rootfs-diff`, and supply `baseline.tar`, `candidate.tar` and `policy.json` before these steps. Archive creation/download belongs to your build pipeline, not to rootfs-diff.

## Preserve the tool's exit status

The comparison can return 0 with ordinary differences, 1 with a configured gate failure, or 2 with an operational failure. Capture the actual executable's status. `go run` and a pipeline ending in `tee` are unsuitable when the exact 0/1/2 result is needed.

```yaml
- name: Review candidate rootfs
  id: rootfs
  shell: bash
  run: |
    mkdir -p artifacts
    exec 3> artifacts/report.partial
    if ./bin/rootfs-diff --format markdown --policy policy.json baseline.tar candidate.tar >&3; then
      status=0
    else
      status=$?
    fi
    exec 3>&-
    printf '%s\n' "$status" > artifacts/exit-code.txt
    printf 'exit_code=%s\n' "$status" >> "$GITHUB_OUTPUT"
    case "$status" in
      0|1)
        mv artifacts/report.partial artifacts/report.md
        printf 'complete=true\n' >> "$GITHUB_OUTPUT"
        cat artifacts/report.md >> "$GITHUB_STEP_SUMMARY"
        ;;
      *)
        printf 'complete=false\n' >> "$GITHUB_OUTPUT"
        printf 'rootfs-diff failed with exit %s; no complete report.\n' "$status" >> "$GITHUB_STEP_SUMMARY"
        ;;
    esac
    exit "$status"

- name: Save completed review report
  if: ${{ always() && steps.rootfs.outputs.complete == 'true' }}
  uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7
  with:
    name: rootfs-review
    path: |
      artifacts/report.md
      artifacts/exit-code.txt
    if-no-files-found: error
```

The comparison remains a failed step for exits 1 and 2. No `continue-on-error` or trailing successful command hides its status. The output descriptor is opened before status capture; a destination-open failure stops the shell instead of masquerading as a tool gate failure. A gate failure still saves a complete report and summary. On exit 2, the partial file is kept separate and is never uploaded as a completed report. Stderr remains in the job log. An interrupted process also cannot turn the step green.

For JSON artifacts, select `--format json`, use a `.json` completed filename, and write only a short status to the job summary. Check exit 0/1 before parsing. Running a separate Markdown comparison for a human summary requires checking that command's status too.

GitHub documents [workflow conditions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax), [job summaries](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands#adding-a-job-summary) and the standard [artifact uploader](https://github.com/actions/upload-artifact). Configure read access if your own source or input archives are private. Do not include credentials or whole firmware build trees in reports/artifacts.

## Repository verification

The [repository workflow](../.github/workflows/ci.yml) runs on pushes to `main` and manual dispatch with Go **1.27.4**. Both Linux and macOS run the same offline formatting/vet/test/build/example entry point and race checks. Linux also runs two 30-second fuzz checks with two workers and four CGO-free cross-builds. Tests include public-package examples and actual executable subprocess checks.

From the repository root after toolchain setup:

```sh
./scripts/verify.sh
go test -race ./...
GOMAXPROCS=2 go test . -run='^$' -fuzz=FuzzScan -fuzztime=30s
GOMAXPROCS=2 go test . -run='^$' -fuzz=FuzzParsePolicy -fuzztime=30s
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/rootfs-diff-linux-amd64 ./cmd/rootfs-diff
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/rootfs-diff-linux-arm64 ./cmd/rootfs-diff
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/rootfs-diff-darwin-amd64 ./cmd/rootfs-diff
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/rootfs-diff-darwin-arm64 ./cmd/rootfs-diff
```

`verify.sh` checks formatting without changing files, runs `go vet ./...` and `go test ./...`, builds the CLI and runs the complete library example. `GOMAXPROCS=2` bounds fuzz worker concurrency; each run still uses the full 30-second budget. To forbid downloads during local checks, use the offline settings in [installation](installation.md#offline-checks) with an already installed toolchain.

Cross-compilation demonstrates build portability, not execution on that target. Native Linux/macOS workflow results supply runtime test evidence for those hosts; local checks alone do not establish a green workflow. The separate pinned Buildroot proof has its own prerequisites and acceptance gate under [issue #8](https://github.com/Azhovan/rootfs-diff/issues/8).
