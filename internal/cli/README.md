# CLI integration

`cmd/rootfs-diff` starts the process and calls `Run`. `Run` accepts a context,
the arguments after the executable name (`argv[0]`), writers for reports and
diagnostics, and optional version and file-opening dependencies. It owns and
closes the inputs it opens, including on failure. It does not close or flush
the supplied writers. A custom opener must return a regular file with `Read`,
`Stat` and `Close` methods, transferring ownership of that file to `Run`.

The command's `--help` lists the supported options. Flags must precede the two
local regular-file inputs. Tar and gzip detection does not depend on filename
extensions. Limits are positive decimal values that fit in `int64`. Standard
input, URLs and input symlinks are unsupported.
Use `./` before a local filename beginning with `-`.

```sh
go build -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff --format json before.tar after.tar.gz > report.json
./bin/rootfs-diff --policy policy.json --ignore-prefix var/log before.tar after.tar
./bin/rootfs-diff --fail-on-change before.tar after.tar
```

Exit 0 means the comparison succeeded and all enabled checks passed. Ordinary
differences alone do not cause failure. Exit 1 means a policy violation, or
changes found with `--fail-on-change`; stdout still contains a complete report.
Exit 2 means invalid usage or input, an exceeded limit, an I/O or rendering
failure, or cancellation. Failures before rendering leave stdout empty. Reports
go to stdout and diagnostics to stderr. A write error, including a closed pipe,
returns 2 and may leave a partial report. Consumers must check the exit status
before using the output.
Cancellation is checked between reads and workflow stages; it cannot interrupt an
arbitrary reader or writer already blocked in a call. The executable handles
interrupt and termination signals using the same cancellation context.

Development builds report `rootfs-diff dev`. To inject trusted build metadata:

```sh
go build -ldflags '-X main.version=example' -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff --version
```

The CLI calls the public packages for scanning, comparison, policy parsing,
policy evaluation and rendering. Ignore rules affect comparison and growth;
required-path checks inspect the complete candidate manifest. Reports include
warnings and standard metadata exclusions even when paths are ignored. See the
[contract](../../docs/v0.1-contract.md) and
[schema](../../docs/report-v1.schema.json) for archive behavior and output fields.
