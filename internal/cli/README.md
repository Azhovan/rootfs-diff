# CLI integration

`cmd/rootfs-diff` is the process entry point. `Run` accepts a context, argument
slice excluding argv[0], output/diagnostic writers, and optional version/opener
injection. It owns and closes opened inputs, including on failure. It never
closes or flushes the supplied writers. An injected opener must return an owned
file with `Read`, `Stat` and `Close`; the opened file must be regular.

The command's `--help` is the authoritative option reference. Flags precede the
two local regular-file inputs; tar/gzip detection does not depend on extensions.
Limits are positive decimal int64 values. Standard input, URLs and input symlinks are unsupported.
Use `./` before a local filename beginning with `-`.

```sh
go build -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff --format json before.tar after.tar.gz > report.json
./bin/rootfs-diff --policy policy.json --ignore-prefix var/log before.tar after.tar
./bin/rootfs-diff --fail-on-change before.tar after.tar
```

Exit 0 means comparison and enabled gates passed. Ordinary differences succeed.
Exit 1 means policy violations or the explicit change gate; stdout still contains
a complete report. Exit 2 means usage, input, limit, I/O, rendering or cancellation
failure. Pre-render failures leave stdout empty. Reports use stdout and diagnostics
use stderr. A downstream write error, including a closed pipe, returns 2 and can
leave a partial report. Consumers must inspect exit status before trusting output.
Cancellation is checked between reads and workflow stages; it cannot interrupt an
arbitrary reader or writer already blocked in a call. The executable handles
interrupt and termination signals using the same cancellation context.

Development builds report `rootfs-diff dev`. To inject trusted build metadata:

```sh
go build -ldflags '-X main.version=example' -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff --version
```

The CLI delegates scanning, comparison, policy parsing/evaluation and rendering
to the public packages. Ignore rules affect comparison and growth; required paths
are checked against the complete candidate manifest. Reports retain warnings and
standard metadata exclusions even when paths are ignored. See the contract and
schema for archive semantics and output fields.
