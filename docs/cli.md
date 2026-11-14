# CLI, policies and ignores

The CLI compares two recorded rootfs archives and applies only the checks you enable.

```text
rootfs-diff [flags] BEFORE.tar[.gz] AFTER.tar[.gz]
```

Both inputs must be local regular files, not symlinks. Gzip is detected by magic bytes, so extensions do not select the decoder. Flags precede the two positional filenames. Write a filename beginning with `-` as `./-filename.tar`; `--` does not make a leading-dash path acceptable. Standard input, URLs and directories are not inputs.

## Flags

| Flag | Value and effect |
|---|---|
| `--format` | `text` (default), `json` or `markdown` |
| `--policy` | Filename of one strict JSON policy |
| `--ignore-path` | Repeatable exact canonical path |
| `--ignore-prefix` | Repeatable canonical path and its descendants |
| `--fail-on-change` | Enable exit 1 for any retained added, removed or modified path |
| `--max-entries` | Positive decimal count, default `100000` per archive |
| `--max-archive-bytes` | Positive decimal decoded bytes, default `8589934592` per archive |
| `--max-file-bytes` | Positive decimal regular payload bytes, default `2147483648` per file |
| `--max-path-bytes` | Positive decimal UTF-8 bytes, default `4096` per entry path or link target |
| `--help` | Print help and exit 0 without input files |
| `--version` | Print version and exit 0 without input files |

Limits have no unit suffixes. Unlike zero-valued Go scan options, explicit CLI overrides must be positive. Use shell redirection to select an output file; there is no output-file flag.

Ordinary builds report `rootfs-diff dev` for `--version`. The supported flag spellings above use `--`; Go's flag parser also accepts a single `-` and `--name=value`. Boolean gates use `--fail-on-change` or `--fail-on-change=false`, rather than a separate `false` argument.

```sh
./bin/rootfs-diff --format markdown baseline.tar candidate.tar.gz > report.md
./bin/rootfs-diff --ignore-path etc/build-id --ignore-prefix var/log baseline.tar candidate.tar
./bin/rootfs-diff --fail-on-change baseline.tar candidate.tar
```

## Policy format

Save the following as `policy.json`, adapting required paths and the byte budget to your project:

```json
{
  "schema_version": 1,
  "required_paths": ["etc/init.d/S40network", "usr/bin/device-agent"],
  "max_total_growth_bytes": 1048576
}
```

```sh
./bin/rootfs-diff --policy policy.json --format json baseline.tar candidate.tar > report.json
```

The policy is one JSON object of at most 1 MiB. `schema_version` and the `required_paths` array are required. `max_total_growth_bytes` is optional; omitting it disables the growth gate, and zero prohibits positive net growth. Its value must be a nonnegative integer representable by `int64`. Equality passes, and shrinking passes. Required paths may be an empty array; duplicate paths have no additional effect.

The parser rejects unknown fields, duplicate object keys, bad types (including null instead of required arrays/integers), unsupported schema versions, trailing JSON, overflowing integers and malformed paths. Policies have no globs, regexes, negation, percentage gates or implicit security rules. Use decimal JSON integers, not octal literals or unit suffixes.

Required paths inspect the complete candidate manifest. An explicitly recorded entry satisfies presence, including a symlink without following its target. An implicit parent directory does not. A path ignored in the diff is still checked for required presence. A missing path produces `required_path`; excess growth produces `max_total_growth_bytes`. Violations sort by rule, then path.

## Ignore boundaries

Ignore paths and prefixes must already be canonical: `etc/device.conf`, not `/etc/device.conf`, `./etc/device.conf` or `etc//device.conf`. Archive entry paths are normalized during scanning; user-supplied ignore and policy paths are validated instead of normalized.

| Rule | Matches | Does not match |
|---|---|---|
| `--ignore-path var/log` | The explicit `var/log` record | `var/log/messages` |
| `--ignore-prefix var/log` | `var/log` and `var/log/messages` | `var/logger` |

Rules apply equally to the baseline and candidate before change detection and growth accounting. Duplicate/unordered rules have the same effect. Warnings are retained even for ignored paths. Ignoring a hardlink target does not remove its aliases; their effective metadata was resolved while scanning the full archive.

## Output and exit codes

| Exit | Meaning | Report |
|---|---|---|
| `0` | Comparison succeeded and enabled checks passed; ordinary differences alone pass | Complete requested report |
| `1` | Policy violation, or changes with `--fail-on-change` | Complete requested report |
| `2` | Invalid arguments/policy, unsupported/corrupt input, limit, I/O/render failure or cancellation | No complete report guaranteed |

Stdout contains the report; stderr contains operational diagnostics. Help and version are informational exceptions to report output and need no input files. No progress text is mixed into reports. Parse, scan and compare failures write no report bytes. Shell redirection may still create an empty file before the command starts. A downstream writer failure may leave partial output and returns 2.

Check the exit status before consuming a JSON report. A created filename or parseable fragment does not prove completion. Avoid piping through another command unless the shell preserves rootfs-diff's status. See the [CI example](ci.md) for saving reports and retaining exits 0, 1 and 2.
