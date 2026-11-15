# Offline synthetic release review

Run from the repository root with the pinned Go 1.27.4 toolchain installed and
active on `PATH`:

```sh
./scripts/demo.sh
```

The script builds and runs the CLI, prints a text report with policy checks
enabled, and checks the expected results. It runs offline without root access,
hardware, Docker, a host tar command, mounting or archive extraction. Go writes
small tar and gzip archives from fixed metadata and file contents. The script
removes its temporary binaries and inputs when it exits. A run typically takes
a few seconds and needs roughly enough disk space for two Go executables.

The baseline has 11 explicitly recorded entries and 27 bytes of regular-file
contents. The candidate has 10 such entries and 31 bytes of regular-file
contents. Three directory records and `var/log/boot.log` are unchanged. The
scanner ignores the root directory record, so it does not appear in either
manifest. Modes below are octal; JSON stores them as decimal integers.

| Path | Controlled difference | Expected fields |
| --- | --- | --- |
| `etc/current` | Symlink `/usr/share/old` becomes `../usr/share/new` | `link_target` |
| `etc/demo.conf` | `mode=A\n` becomes same-size `mode=B\n` | `sha256` |
| `etc/init.d/S90demo` | Six-byte startup script is removed | removed, with complete before metadata |
| `usr/bin/demo-agent` | Mode `0755` becomes `04755`, bytes unchanged | `mode` |
| `usr/share/alias` | Hardlink to `usr/share/payload` reflects that file's growth and content change | `size`, `sha256` |
| `usr/share/chain` | Hardlink target changes from `usr/share/alias` to `usr/share/payload`; it also reflects the payload's growth and content change | `size`, `sha256`, `link_target` |
| `usr/share/payload` | `data\n` becomes `data grows now\n`, preserving UID 12/GID 34 and mode `0640` | `size`, `sha256` |

The expected result is one removal, six modifications, no additions and four
unchanged entries. The only positive-growth record is `usr/share/payload`, at
+10 bytes. Removing the six-byte startup script brings net growth to +4 bytes.
Neither hardlink counts the payload bytes again. The hardlink headers
deliberately contain inconsistent permissions and ownership; the scanner uses
the metadata of the regular file at the end of each link chain instead.

The demo checks complete manifests, changes, hashes, totals and policy results
against expected values written in `expected.go`. Those values were written
independently of the scanner and comparator. It also checks:

- Exit 0 for ordinary differences, baseline versus itself, uncompressed tar
  versus equivalent gzip input with reordered entries, a growth limit equal to
  +4, and `--fail-on-change` when all changes are ignored.
- Exit 1 with a complete report for `--fail-on-change` and the supplied policy.
  `policy.json` produces two sorted violations: growth of 4 bytes exceeds 0, and the
  required startup script is absent. Ignoring `usr/share` changes net growth to
  -6 and leaves only the missing-path violation. Ignoring the missing required
  path itself does not satisfy that policy.
- Exit 2 with a diagnostic and no report bytes for a path containing `..`, a
  truncated header, a corrupt gzip checksum, a duplicate policy key, and a
  one-byte per-file limit.
- Byte-identical text, JSON and Markdown output across repeated runs, identical
  JSON for equivalent archive encodings, agreement between library and CLI
  results, and repeatable archive generation. The equivalent gzip inputs
  deliberately differ in timestamps, user/group names and entry order.

Every report declares standard metadata exclusions. These fixtures contain no
vendor PAX metadata, so their warning arrays are empty.

To retain the generated inputs for manual inspection, choose an empty temporary
directory or a location beneath ignored `artifacts/`:

```sh
go build -o ./bin/rootfs-diff ./cmd/rootfs-diff
go run ./examples/synthetic/cmd/demo -cli ./bin/rootfs-diff -out ./artifacts/synthetic
./bin/rootfs-diff --format json ./artifacts/synthetic/before.tar ./artifacts/synthetic/after.tar
./bin/rootfs-diff --policy ./artifacts/synthetic/policy.json ./artifacts/synthetic/before.tar ./artifacts/synthetic/after.tar
# The preceding policy command deliberately exits 1.
rm -rf ./artifacts/synthetic
```

The same checks run under `go test ./integration/...` and `./scripts/verify.sh`.
The slower [Buildroot demonstration](../buildroot/README.md) runs separately;
normal verification does not download or build Buildroot. This synthetic demo
checks archive comparison behavior. It does not validate real firmware or
establish bootability, runtime correctness, OTA compatibility or security.
