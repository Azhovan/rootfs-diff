# Public Go library example

Run this self-contained example from the repository root:

```sh
mkdir -p artifacts
go run ./examples/library > artifacts/library-report.json
go test ./examples/library
```

The [example source](main.go) imports the public packages
`github.com/Azhovan/rootfs-diff` and `github.com/Azhovan/rootfs-diff/report`
from a separate package. It creates tiny tar archives in memory, so you do not
need firmware images, a system `tar` command, downloads or generated fixtures.
It reads archive entries without extracting or executing them. In your own
application, replace archive creation with readers for your tar or tar.gz files.

The example calls `Scan` for each archive, compares the resulting manifests with
`Compare`, reads a policy with `ParsePolicy`, checks it with `Evaluate` and writes
the complete schema-version-1 report as JSON with `report.Write`. It prints the
report to stdout. This example program exits with code 1 only when an operation
fails; policy violations appear as report data. The rootfs-diff CLI uses its own
[exit codes](../../docs/cli.md#output-and-exit-codes) to signal failed checks.

The [test](main_test.go) checks these independently defined expectations:

- `etc/device.conf` changes mode `0640` to `06750`, content `hello` to `hello\n`,
  and size 5 to 6 bytes. Numeric UID/GID remain 100/200.
- `usr/bin/agent` changes symlink target from `../lib/agent-v1` to
  `../lib/agent-v2`. The targets are recorded, never followed.
- `etc/build-id` changes but is ignored by exact path in both archives. It still
  satisfies the policy's required-path check, which uses the complete candidate
  manifest.
- After ignores, the summary has two entries per side and two modifications.
  Regular-file contents total 5 bytes before and 6 bytes after, for net growth of
  1 byte. This equals the one-byte growth limit, so the check passes. There are
  no violations or metadata warnings. The report still lists all standard
  metadata exclusions.

Your application owns its readers and writers. A `bytes.Reader` does not need to
be closed; if you use files, open and close them in your application and handle
close errors. `Scan` checks for cancellation between reads, but it cannot
interrupt a blocked reader. Library operations do not change input models or
choose process exit codes. You can make independent calls concurrently with
separate readers and writers, provided you do not change the inputs at the same
time. A writing error can leave a partial report, so check the returned error
before using the output. The example also shows how to inspect error categories
with `errors.Is` and details with `errors.As`, while preserving underlying errors.

See the [library reference](../../docs/library.md),
[installation/module access](../../docs/installation.md) and
[behavior contract](../../docs/v0.1-contract.md). The upstream module is public;
you only need the authentication instructions if you use a private copy or fork.
