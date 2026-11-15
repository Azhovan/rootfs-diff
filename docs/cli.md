# CLI, policies and ignores

The command-line tool compares a baseline root filesystem archive with a candidate archive. It reports changes and applies any checks you enable.

```text
rootfs-diff [flags] BEFORE.tar[.gz] AFTER.tar[.gz]
```

Supply two local regular files; symlinks, directories, URLs and standard input are not supported. The tool detects gzip from the file's bytes, so the filename extension does not determine how it is read.

Put all flags before the two filenames. If a filename begins with `-`, write it as `./-filename.tar`. Adding `--` before a filename does not allow a path that starts with `-`.

## Flags

| Flag | Value and effect |
|---|---|
| `--format` | `text` (default), `json` or `markdown` |
| `--policy` | Read one policy from a JSON file; see [policy format](#policy-format) |
| `--ignore-path` | Ignore one exact canonical path; may be repeated |
| `--ignore-prefix` | Ignore a canonical path and its descendants; may be repeated |
| `--fail-on-change` | Exit with code 1 if any path is added, removed or modified after ignores are applied |
| `--max-entries` | Maximum entries per archive; default `100000` |
| `--max-archive-bytes` | Maximum decoded archive bytes, including headers and padding; default `8589934592` per archive |
| `--max-file-bytes` | Maximum uncompressed contents of one regular file, in bytes; default `2147483648` |
| `--max-path-bytes` | Maximum UTF-8 bytes in each entry path or link target; default `4096` |
| `--help` | Print help and exit 0 without input files |
| `--version` | Print version and exit 0 without input files |

Write limits as positive decimal integers, without unit suffixes. A command-line limit of zero is invalid, even though zero selects the default in Go scan options. To save a report, redirect stdout to a file as shown below.

Source builds report `rootfs-diff dev` for `--version`. The flag spellings above use `--`; Go's flag parser also accepts a single `-` and values written as `--name=value`. To disable the change check explicitly, use `--fail-on-change=false`. A separate `false` argument is treated as a filename.

```sh
./bin/rootfs-diff --format markdown baseline.tar candidate.tar.gz > report.md
./bin/rootfs-diff --ignore-path etc/build-id --ignore-prefix var/log baseline.tar candidate.tar
./bin/rootfs-diff --fail-on-change baseline.tar candidate.tar
```

## Policy format

Save the following as `policy.json`, choosing required paths and a growth limit that suit your project:

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

The policy file must contain one JSON object and be no larger than 1 MiB. Both `schema_version` and the `required_paths` array are required. You can use an empty array when no paths are required; listing a path more than once has no additional effect.

`max_total_growth_bytes` is optional. Omit it to disable the growth check, or set it to zero to prohibit any positive net growth. The value must be a nonnegative integer that fits in an `int64`. Growth equal to the limit passes, as does a reduction in total size. See [growth accounting](semantics.md#growth-accounting) for how net growth is calculated.

The parser rejects unknown fields, duplicate object keys, incorrect types (including `null` in place of required arrays or integers), unsupported schema versions, trailing JSON, overflowing integers and malformed paths. Use decimal JSON integers, without octal notation or unit suffixes. Policies support exact required paths and a net growth limit; they have no wildcard patterns, regular expressions, negation, percentage limits or inferred security rules.

Required-path checks use the complete list of entries scanned from the candidate archive, even for paths ignored in the comparison. Any explicitly recorded entry satisfies the check. A symlink counts as present without checking its target; a parent directory implied by a child's path does not count unless the archive records it separately.

A missing path produces a `required_path` violation. Growth above the limit produces a `max_total_growth_bytes` violation. Reports sort violations by rule, then path.

## Ignore boundaries

Ignore paths, prefixes and required paths must use canonical spelling: a path relative to the root filesystem, with no leading or trailing slash, empty segments, `.` or `..`. For example, use `etc/device.conf`, not `/etc/device.conf`, `./etc/device.conf` or `etc//device.conf`. The scanner normalizes archive paths, but the tool rejects noncanonical paths supplied in ignore rules or policies.

| Rule | Matches | Does not match |
|---|---|---|
| `--ignore-path var/log` | The explicit `var/log` record | `var/log/messages` |
| `--ignore-prefix var/log` | `var/log` and `var/log/messages` | `var/logger` |

Ignore rules apply to both the baseline and candidate before changes and growth are calculated. The order of the rules does not matter, and duplicate rules have no additional effect. Warnings remain visible for ignored paths. Ignoring a hardlink's target leaves the other linked paths in the comparison; their content and metadata were already resolved from the full archive during scanning.

## Output and exit codes

| Exit | Meaning | Report |
|---|---|---|
| `0` | Comparison succeeded and all enabled checks passed; differences alone do not fail | Complete report in the selected format |
| `1` | A policy check failed, or changes were found with `--fail-on-change` | Complete report in the selected format |
| `2` | Invalid arguments or policy, unsupported or corrupt input, exceeded limit, I/O or report-writing failure, or cancellation | Output may be missing or incomplete |

The command writes reports to stdout and error messages to stderr. It does not mix progress messages into reports. `--help` and `--version` write their information to stdout and do not require input files.

If parsing, scanning or comparison fails, the command writes no report bytes, although shell redirection may already have created an empty output file. A failure while writing the report can leave partial output and returns exit code 2.

Check the exit code before using a JSON report. A file's existence, or a fragment that can be parsed, does not establish that the report is complete. If you pipe the output through another command, make sure you still capture rootfs-diff's exit code. The [CI example](ci.md) shows how to save reports while preserving codes 0, 1 and 2.
