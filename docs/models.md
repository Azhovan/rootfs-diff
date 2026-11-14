# Models and JSON schema version 1

The root `rootfsdiff` package provides models, `Validate` methods,
`ScanOptions.WithDefaults`, `ValidatePath`, `MetadataExclusions`, and typed errors.
The [behavior contract](v0.1-contract.md) remains the semantic reference.

## Component boundaries

Scanning exposes:

```go
Scan(ctx context.Context, r io.Reader, opts ScanOptions) (Manifest, error)
```

Comparison exposes:

```go
Compare(before, after Manifest, opts CompareOptions) (Diff, error)
```

Policy operations expose:

```go
ParsePolicy(r io.Reader) (Policy, error)
Evaluate(after Manifest, diff Diff, policy Policy) ([]Violation, error)
```

The public `report` package imports root `rootfsdiff` and implements
`Write(io.Writer, rootfsdiff.Report, Format) error`, with string formats `text`,
`json`, and `markdown`. Core never imports it. `internal/cli` owns an injectable
`Run` function, process diagnostics and exit-code selection;
`cmd/rootfs-diff/main.go` is the thin process entry point. Public component and
[complete library examples](../examples/library/README.md) compile and execute
under `go test ./...`.

Core operations must validate caller-built inputs, avoid mutation, printing and
exits, and retain underlying errors. The scanner does not close its caller-owned
reader. Cancellation checks occur between bounded reads and cannot interrupt an
arbitrary reader already blocked inside `Read`. Independent calls share no
mutable options. Concurrent caller mutation of input slices or entries is not
supported.

## Value and validation rules

Every data model (`Entry`, `Manifest`, `Change`, `Summary`, `Growth`, `Diff`,
`Policy`, `Violation`, `Warning`, `Report`) and options type has `Validate`.
Validation reads inputs without sorting or rewriting them. All byte sizes,
counters, limits, UID/GID and device numbers use `int64`; mode uses `uint32`
restricted to `07777`. Tar numeric IDs are preserved, including signed IDs,
without host name lookup. Sizes/counts/device numbers are nonnegative and checked
aggregate payload sums cannot overflow `int64`. JSON integer overflow, fractional
numbers and negative unsigned modes fail during Go JSON decoding.

Canonical model paths are nonempty UTF-8 POSIX root-relative strings with no
empty, `.` or `..` segment. They have no leading or trailing slash. Backslash,
glob characters and control characters are literal filename data. Model
validation imposes no default scan length cap; the scanner validates decoded
paths against the selected options before normalization. Symlink targets are arbitrary valid
UTF-8 preserved verbatim, including absolute paths, `..` and an empty target.
Hardlink targets must be canonical paths.

`Entry` uses this fixed field order, also used for each change side:

| JSON field | Representation |
|---|---|
| `path` | Canonical string |
| `kind` | `regular`, `directory`, `symlink`, `hardlink`, `char_device`, `block_device`, `fifo` |
| `mode` | Decimal integer representing the `07777` mask; human reports use octal |
| `uid`, `gid` | Signed numeric tar ownership |
| `size` | Regular or effective hardlink size, otherwise zero |
| `sha256` | Lowercase 64-digit hex for regular/hardlink, otherwise empty string |
| `link_target` | Symlink verbatim target or hardlink canonical target, otherwise empty string |
| `device_major`, `device_minor` | Nonnegative device values, otherwise zero |

A hardlink's effective mode, UID, GID, size and SHA-256 must equal its terminal
regular entry. `Manifest.Validate` checks target existence, cycles, endpoint and
effective metadata, as well as unique paths and explicit non-directory ancestors.
Implicit parents are valid and are not synthesized. Caller manifests may have any
entry/warning order; scanner-produced entries sort by path. Only regular entries
contribute payload bytes, so hardlink aliases never multiply totals. Changes are
partial views: their embedded entries validate independently without demanding
hardlink targets that may be absent after filtering.

`ScanOptions` fields are `MaxEntries`, `MaxArchiveBytes`, `MaxFileBytes`, and
`MaxPathBytes`. Zero means the contract defaults of 100,000, 8 GiB, 2 GiB and
4,096 respectively. `WithDefaults` returns an independent effective value.
Negatives fail, positive overrides are accepted up to `int64` maximum; cross-limit
relationships need no extra restriction. `CompareOptions` contains `IgnorePaths`
and `IgnorePrefixes`; every value must be canonical. Duplicate and unordered
ignore rules or required policy paths are accepted and have no extra effect.

## JSON layout and ordering

[report-v1.schema.json](report-v1.schema.json) fixes structural schema version 1,
including reusable definitions for models. Go `Validate` additionally checks
cross-field consistency, ordering, uniqueness, hardlinks and arithmetic. Ordinary
`encoding/json` decoding is not a strict policy parser or a substitute for
validation; `ParsePolicy` enforces the contract's duplicate/unknown-key,
trailing-input and 1 MiB restrictions. The structural schema rejects unknown
fields; callers using Go decoding must separately control unknown fields if they
need that property.

Every model slice marshals as `[]` when empty, including nested `changed_fields`,
policy paths and compare options. Marshaling does not validate or mutate values.
`before` and `after` are always present, with an absent side encoded as `null`.
Optional policy `max_total_growth_bytes` is omitted when its pointer is nil; a
pointer to zero enables a no-positive-growth gate. `Violation.limit` and `actual`
are explicit null for required-path failures. No timestamp, host filename or
run-dependent field exists.

The top-level report order is:

1. `schema_version` (always 1)
2. `summary`
3. `changes`
4. `positive_growth`
5. `violations`
6. `warnings`
7. `metadata_exclusions`

Summary order is `before_entries`, `after_entries`, `added`, `removed`, `modified`,
`unchanged`, `before_payload_bytes`, `after_payload_bytes`, `net_growth_bytes`.
Entry counts describe both retained sides after ignores. Before count equals
removed + modified + unchanged; after count equals added + modified + unchanged.
Payload totals count retained regular entries only, and net growth is after minus
before. Diff validation checks change counts and changed payload contributions,
including equal residual bytes for unchanged entries.

Changes sort by canonical path and use `path`, `kind`, `before`, `after`,
`changed_fields`. Kinds are `added`, `removed`, `modified`. Added/removed changes
carry the full present side and an empty changed-field array. Modified changes
carry both sides and every unequal compared field in this order: `kind`, `mode`,
`uid`, `gid`, `size`, `sha256`, `link_target`, `device_major`, `device_minor`.
Type changes are single modified records.

`positive_growth` records have `path`, `bytes`. They include each positive regular
payload delta from the changes, including type transitions, and sort by descending
bytes then ascending path. JSON retains the full list; human renderers show at
most ten growth entries while rendering all changes.

Violations use `rule`, `path`, `limit`, `actual`, sorted uniquely by rule then
path. `required_path` has a canonical path and null numeric fields.
`max_total_growth_bytes` has empty path and nonnegative limit with actual greater
than limit; report actual must equal summary net growth. Evaluation checks the
complete candidate manifest for required paths, regardless of comparison ignores.

Warnings use `source`, `path`, `code`, `key`. Code is `uncompared_metadata`; key is
nonempty valid UTF-8 and no value is recorded. A warning path is canonical, with
one warning-only exception: `.` identifies metadata on an ignored root directory
record. Root warnings remain even though no root entry is retained or synthesized.
All actual entry, hardlink target, ignore and required-path rules are unchanged;
`.` is invalid for those paths. Manifest sources are empty strings, and each
nonroot warning identifies an explicit entry. Comparison/report sources are
`before` or `after` even for ignored paths. Output warnings sort uniquely by path,
code, key, source using Go string order (`after` precedes `before` on a final tie).
Warnings do not prove runtime consequences.

`metadata_exclusions` is the fixed sorted list returned as a fresh slice by
`MetadataExclusions()`: `acls`, `directory_sizes`, `file_capabilities`,
`selinux_labels`, `timestamps`, `unrecognized_pax_metadata`, `user_group_names`,
`xattrs`. Every report includes it. Order, compression and padding are archive
representation details outside comparison; hardlink header metadata is ignored in
favor of terminal regular metadata.

Schema-version changes require coordinated model, example and consumer updates.
Version 1 output fields and enum meanings are fixed by these examples. Human
renderers must escape untrusted control characters and Markdown/HTML syntax.

## Executable fixtures

- [Complete report](../testdata/models/report-v1.json): before count 3, after count
  3, one added/removed/modified and one unchanged; payload 8 to 9 bytes, net +1.
  `etc/device.conf` changes from mode `0640` (JSON 416) to `06750` (JSON 3560),
  size 2 to 4 and a changed SHA-256. `etc/new` adds 2 bytes and `obsolete` removes
  3. Positive growth lists the two +2 paths, tied by path. A zero-byte growth gate
  fails, as does a missing required `usr/bin/agent`.
- [Empty report](../testdata/models/empty-report-v1.json): all counters zero,
  explicit empty arrays and the same exclusions.
- [Valid manifest](../testdata/models/valid-manifest.json): all seven entry kinds,
  numeric IDs/special modes, two forward hardlink aliases, a verbatim symlink
  target and an uncompared-metadata warning.
- [Root warning manifest](../testdata/models/root-warning-manifest.json) and
  [report](../testdata/models/root-warning-report-v1.json): root-record metadata
  warnings are retained without inventing an entry, change or payload byte.
- [Invalid manifests](../testdata/models/invalid-manifests.json): malformed path,
  negative size, unknown kind, missing link, aggregate overflow and duplicate path.
- [Policy](../testdata/models/policy-v1.json): version 1 with required paths and a
  nonnegative explicit growth limit.

Fixture expectations were authored without production scanning or comparison.
Tests decode, validate and round-trip reports with exact fields/order and inspect
the report's independently expected modes, changed fields and growth.

## Errors

Use `errors.Is(err, ErrInvalidInput)`, `ErrUnsupportedInput`, or
`ErrLimitExceeded` for input categories. `*Error` carries operation, archive path,
kind and underlying error. `*LimitError` carries budget name, maximum, first
observed excess and optional underlying error; use `errors.As` for details.
An empty `Error.Kind` adds context without assigning an input category. Standard
I/O errors and cancellation remain reachable through `Unwrap`. Model validation
failures are invalid input; they do not claim an archive parser has run.
