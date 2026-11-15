# CI usage

You can run rootfs-diff in a GitHub Actions workflow using the steps below. First set up Go and the source checkout, build `./bin/rootfs-diff`, and provide `baseline.tar`, `candidate.tar` and `policy.json`. Your build pipeline must create or download the archives before the comparison runs.

## Fail CI for a missing required path

To require the network startup path in the candidate archive, save this policy as `policy.json`:

```json
{
  "schema_version": 1,
  "required_paths": ["etc/init.d/S40network"]
}
```

Paths are relative to the root filesystem and have no leading `/`. If the candidate archive does not explicitly record the required path, the command reports a `required_path` violation and exits with **code 1 and a complete report**. An ignore rule does not suppress this check. Any recorded entry at the path passes, including a symlink; the check does not follow links or establish that the startup script will run correctly. This policy sets no growth limit.

For the [Buildroot example's archive pair](../examples/buildroot/README.md#observed-execution), this policy returns exit code 1 with one `required_path` violation. The full example uses a different policy that also sets `max_total_growth_bytes` to `409600`, so its report lists two violations.

In that comparison, `libexample.so` grows by 420 KiB (430,080 bytes). The 17-byte startup stub is also removed, leaving net growth of 430,063 bytes. Use the report's `net_growth_bytes` when choosing a growth limit: it accounts for both increases and reductions across the comparison.

## Preserve the tool's exit status

The command returns exit code 0 when the comparison and all enabled checks pass, 1 when a check fails, and 2 when input or an operation fails. Differences alone return 0. Run the built executable and capture its exit code directly. `go run` does not preserve the command's exact exit code, and a pipeline ending in `tee` can report `tee`'s status instead.

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

The step fails for exit codes 1 and 2 and for an interrupted process. It saves a complete report and adds it to the job summary for codes 0 and 1. For code 2, it keeps any partial output separate and does not upload it as a completed report. Error messages remain in the job log.

The shell opens the output file before capturing the command's status. If it cannot open the destination, the step stops rather than mistaking that failure for a policy violation. The final `exit "$status"` preserves the comparison's result; adding `continue-on-error` or ending with a successful command would hide a failure.

To save JSON, select `--format json`, change the completed report's extension to `.json`, and write a short status message to the job summary instead of the JSON. Parse the report only after checking for exit code 0 or 1. If you run a separate Markdown comparison for the summary, check that command's exit code too.

See GitHub's documentation for [workflow conditions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax), [job summaries](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands#adding-a-job-summary) and [uploading artifacts](https://github.com/actions/upload-artifact). Configure read access if your source or input archives are private. Keep credentials and firmware build trees out of report artifacts.

## Repository verification

The [repository workflow](../.github/workflows/ci.yml) runs on pushes to `main` and can be started manually. It uses Go **1.27.4**. On both Linux and macOS, it runs the offline verification script and race checks. The tests include public-package examples and subprocess tests of the built executable. Linux also runs two 30-second fuzz checks with two workers, then cross-compiles for four platforms with CGO disabled.

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

`verify.sh` checks formatting without changing files, runs `go vet ./...` and `go test ./...`, builds the command and runs the complete library example. `GOMAXPROCS=2` limits the number of concurrent fuzz workers; each fuzz check still runs for the full 30 seconds. To disable downloads during local checks, install the toolchain first and use the [offline settings](installation.md#offline-checks).

A successful cross-build shows that the command compiles for that platform. Runtime tests run on the native Linux and macOS workflow runners; check the workflow results as well as local results. The [synthetic example](../examples/synthetic/README.md) provides a small comparison you can reproduce. The [Buildroot demonstration](../examples/buildroot/README.md) has separate prerequisites and is not part of the routine workflow. It checks archive differences without booting or running the firmware.
