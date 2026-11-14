# Offline synthetic release review

Run from the repository root with the pinned Go 1.27.4 toolchain installed and
active on `PATH`:

```sh
./scripts/demo.sh
```

This builds and runs the actual CLI, prints a gated text report, and checks all
expected results. It requires no network, root, hardware, Docker, host tar,
mounting, or archive extraction. Go writes small tar and gzip archives directly
from fixed metadata and literal payloads. Temporary binaries and inputs are
removed on exit. Runtime and disk costs are small, typically a few seconds and
the size of two Go executables.

The baseline has 11 explicit entries and 27 regular payload bytes. The candidate
has 10 explicit entries and 31 regular payload bytes. Three explicit directories
and `var/log/boot.log` are unchanged. The ignored root directory record is not an
entry in either manifest. Modes below are octal; JSON stores mode numbers as
decimal integers.

| Path | Controlled difference | Expected fields |
| --- | --- | --- |
| `etc/current` | Symlink `/usr/share/old` becomes `../usr/share/new` | `link_target` |
| `etc/demo.conf` | `mode=A\n` becomes same-size `mode=B\n` | `sha256` |
| `etc/init.d/S90demo` | Six-byte startup script is removed | removed, with complete before metadata |
| `usr/bin/demo-agent` | Mode `0755` becomes `04755`, bytes unchanged | `mode` |
| `usr/share/alias` | Hardlink to payload inherits its growth and content change | `size`, `sha256` |
| `usr/share/chain` | Hardlink target changes from alias to payload, with inherited growth/content | `size`, `sha256`, `link_target` |
| `usr/share/payload` | `data\n` becomes `data grows now\n`, preserving UID 12/GID 34 and mode `0640` | `size`, `sha256` |

Expect one removal, six modifications, no additions, and four unchanged entries.
The single positive-growth record is `usr/share/payload` at +10 bytes. Removing
the six-byte startup script makes net growth +4 bytes. Neither hardlink counts
payload bytes again. Deliberately inconsistent hardlink header permissions and
ownership are ignored in favor of terminal regular-entry metadata.

The demo checks hand-authored full manifests, changes, hashes, totals, and policy
results in `expected.go`. Those expectations do not call the scanner/comparator
to create expected results. It also checks:

- Exit 0 for ordinary differences, baseline versus itself, raw versus reordered
  gzip equivalents, a growth limit equal to +4, and a change gate with all
  changes ignored.
- Exit 1 with a complete report for `--fail-on-change` and the supplied policy.
  `policy.json` expects two sorted violations: growth 4 exceeds 0, and the
  required startup script is absent. Ignoring `usr/share` changes net growth to
  -6 and leaves only the missing-path violation. Ignoring the missing required
  path itself does not satisfy that policy.
- Exit 2 with a diagnostic and no report bytes for a traversal path, truncated
  header, corrupt gzip checksum, duplicate policy key, and a one-byte file cap.
- Byte-identical repeated text/JSON/Markdown output, byte-identical JSON across
  equivalent archive encodings, full library/CLI agreement, and deterministic
  archive generation. Timestamp, user/group name, and entry-order differences
  are deliberately present in the equivalent gzip inputs.

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

The same assertions run under `go test ./integration/...` and normal
`./scripts/verify.sh`. The slow, real Buildroot demonstration is separate under
`examples/buildroot/`; normal verification never downloads or builds Buildroot.
Synthetic results prove these artifact semantics, not real firmware validation,
bootability, runtime correctness, OTA compatibility, or a security verdict.
