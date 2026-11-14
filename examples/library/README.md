# Public Go library example

From the repository root:

```sh
mkdir -p artifacts
go run ./examples/library > artifacts/library-report.json
go test ./examples/library
```

The [example source](main.go) is a separate package importing
`github.com/Azhovan/rootfs-diff` and `github.com/Azhovan/rootfs-diff/report`. It
creates tiny tar streams in memory so it needs no firmware images, host tar
command, downloads or generated fixtures. It never extracts archive entries or
executes their contents. In a real consumer, replace the fixture creation with
opened tar or tar.gz readers.

The example calls `Scan` twice, `Compare`, strict `ParsePolicy`, `Evaluate` and
`report.Write` with the complete schema-version-1 report fields. It prints JSON
to stdout. This small example returns process exit 1 only if an operation fails;
it renders policy violations as data. The product CLI's documented 0/1/2 gates
are separate.

Expected findings are independently checked by [the test](main_test.go):

- `etc/device.conf` changes mode `0640` to `06750`, content `hello` to `hello\n`,
  and size 5 to 6 bytes. Numeric UID/GID remain 100/200.
- `usr/bin/agent` changes symlink target from `../lib/agent-v1` to
  `../lib/agent-v2`. The targets are recorded, never followed.
- `etc/build-id` changes but is ignored by exact path on both sides. It still
  satisfies the policy's required-path check against the full candidate.
- The summary has two retained entries per side and two modifications. Regular
  payload totals are 5 and 6 bytes, net +1 byte. A one-byte growth budget passes
  at equality. There are no violations or metadata warnings, and the report
  still declares every standard metadata exclusion.

Readers and writers belong to the application. `bytes.Reader` needs no close;
if using files, the caller opens/closes them and handles close errors. Scan
checks cancellation between reads but cannot unblock an arbitrary blocked
reader. Core operations do not mutate input models or choose process exits.
Independent calls are safe with independent I/O and no concurrent caller
mutation. A writer failure can leave partial report output; check the returned
error before consuming it. The example preserves underlying errors and shows
`errors.Is` categories and `errors.As` details for invalid input and limits.

See the [library reference](../../docs/library.md),
[installation/module access](../../docs/installation.md) and
[behavior contract](../../docs/v0.1-contract.md). The upstream module is public;
authentication guidance for private copies/forks is optional.
