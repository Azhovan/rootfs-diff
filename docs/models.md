# Models and JSON schema version 1

The root `rootfsdiff` package defines the data passed between scanning,
comparison, policy evaluation and reporting. It also provides `Validate`
methods, `ScanOptions.WithDefaults`, `ValidatePath`, `MetadataExclusions` and
typed errors. This page describes those models and their JSON representation;
the [behavior contract](v0.1-contract.md) defines the required behavior.

## Component boundaries

Use `Scan` to read an archive into a manifest, a list of entries and their metadata:

```go
Scan(ctx context.Context, r io.Reader, opts ScanOptions) (Manifest, error)
```

Use `Compare` to find changes between a baseline and candidate manifest:

```go
Compare(before, after Manifest, opts CompareOptions) (Diff, error)
```

Use `ParsePolicy` to read a policy, then `Evaluate` to check the candidate and diff:

```go
ParsePolicy(r io.Reader) (Policy, error)
Evaluate(after Manifest, diff Diff, policy Policy) ([]Violation, error)
```

The public `report` package imports `rootfsdiff` and provides
`Write(io.Writer, rootfsdiff.Report, Format) error`, with string formats `text`,
`json`, and `markdown`. The root package does not depend on the renderer.
`internal/cli` contains the injectable `Run` function and handles diagnostics
and exit codes. `cmd/rootfs-diff/main.go` starts the process and calls it.
The public API examples and
[complete library examples](../examples/library/README.md) compile and execute
under `go test ./...`.

Library operations validate inputs built by callers without changing them. They
do not print or exit the process, and returned errors preserve their underlying
causes. The caller owns the scanner's reader and is responsible for closing it.
The scanner checks cancellation between bounded reads, but cannot interrupt an
arbitrary reader already blocked inside `Read`. Independent calls share no
mutable options. Callers must not modify input slices or entries while an
operation is using them.

## Value and validation rules

Every data model (`Entry`, `Manifest`, `Change`, `Summary`, `Growth`, `Diff`,
`Policy`, `Violation`, `Warning`, `Report`) and options type has a `Validate`
method. Validation reads the input without sorting or rewriting it.

Byte sizes, counters, limits, UID/GID and device numbers use `int64`. Mode uses
`uint32`, restricted to the permission and special bits in `07777`. Numeric tar
IDs retain their signed values and are never resolved to host user or group
names. Sizes, counts and device numbers must be nonnegative; payload totals are
checked for `int64` overflow. Go's JSON decoder rejects integers that overflow
their destination types, fractional numbers in integer fields, and negative modes.

Model paths must already be canonical: nonempty UTF-8 strings relative to the
archived filesystem root, using POSIX `/` separators. They cannot have a leading
or trailing slash, an empty segment, or a `.` or `..` segment. Backslashes, glob
characters and control characters are literal filename data. Model validation
does not impose the scanner's default length limit; the scanner checks decoded
paths against its selected limit before normalization. Symlink targets may be
any valid UTF-8 string and are preserved exactly, including absolute targets,
`..` and an empty target. Hardlink targets must be canonical paths.

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

A hardlink's effective mode, UID, GID, size and SHA-256 must match the regular
file at the end of its link chain. `Manifest.Validate` checks that targets exist,
chains have no cycles and end at regular files, and effective metadata matches.
It also checks path uniqueness and rejects an explicitly recorded non-directory
that is a parent of another entry. Missing parent directory records are valid;
validation does not create them.

Caller-built manifests may list entries and warnings in any order. The scanner
sorts its entries by path. Only regular entries contribute payload bytes, so
hardlinks do not count the same contents again. A change contains only part of
the original manifest. Its embedded entries are therefore validated individually,
without requiring the entries named by hardlink targets to remain in the diff.

`ScanOptions` fields are `MaxEntries`, `MaxArchiveBytes`, `MaxFileBytes`, and
`MaxPathBytes`. Zero selects the respective defaults of 100,000 entries, 8 GiB
per archive, 2 GiB per regular file and 4,096 bytes per path or link target.
`WithDefaults` returns a separate options value with those defaults filled in.
Negative values are invalid; positive values up to the `int64` maximum are
accepted. The limits need not have any particular relationship to one another.

`CompareOptions` contains `IgnorePaths` and `IgnorePrefixes`; every path must be
canonical. Ignore rules and required policy paths may appear in any order.
Duplicates are accepted and have no additional effect.

## JSON layout and ordering

[report-v1.schema.json](report-v1.schema.json) defines the version 1 JSON
structure, including reusable model definitions. The Go `Validate` methods also
check consistency between fields, ordering, uniqueness, hardlinks and arithmetic.
Decoding with `encoding/json` does not replace validation or strict policy
parsing. Use `ParsePolicy` to reject duplicate or unknown keys, trailing input,
and policies larger than 1 MiB. The JSON schema also rejects unknown fields;
callers decoding directly in Go must configure that check separately if needed.

Every empty model slice is encoded as `[]`, including nested `changed_fields`,
policy paths and compare options. JSON encoding does not validate or modify
values. Both `before` and `after` are always present, with an absent side encoded
as `null`. The optional policy field `max_total_growth_bytes` is omitted when
its pointer is nil; a pointer to zero enables a check that forbids positive net
growth. For required-path violations, `limit` and `actual` are explicitly `null`.
Reports contain no timestamp, host filename or field that varies between runs.

The top-level report order is:

1. `schema_version` (always 1)
2. `summary`
3. `changes`
4. `positive_growth`
5. `violations`
6. `warnings`
7. `metadata_exclusions`

Summary fields appear in this order: `before_entries`, `after_entries`, `added`,
`removed`, `modified`, `unchanged`, `before_payload_bytes`, `after_payload_bytes`,
`net_growth_bytes`. Entry counts describe the baseline and candidate after ignore
rules are applied. The baseline count equals removed + modified + unchanged;
the candidate count equals added + modified + unchanged. Payload totals count
only retained regular files, and net growth is after minus before. Diff
validation checks change counts and payload contributions. After subtracting
the bytes accounted for by changes, both sides must have the same remaining
payload total for unchanged entries.

Changes are sorted by canonical path and contain `path`, `kind`, `before`,
`after`, `changed_fields`, in that field order. The change kinds are `added`,
`removed` and `modified`. Additions and removals carry the complete entry on the
present side and an empty changed-field array. Modifications carry both sides
and list every unequal compared field in this order: `kind`, `mode`,
`uid`, `gid`, `size`, `sha256`, `link_target`, `device_major`, `device_minor`.
A change in entry kind is reported as one modified record.

Each `positive_growth` record contains `path` and `bytes`. The list includes
every positive change in regular-file payload size, including entries that
change from another kind to a regular file. Records are sorted by descending
bytes, then ascending path. JSON includes the full list; text and Markdown show
at most ten growth entries while still including all changes.

Violations contain `rule`, `path`, `limit` and `actual`. They are unique and
sorted by rule, then path. A `required_path` violation has a canonical path and
null numeric fields. A `max_total_growth_bytes` violation has an empty path and
a nonnegative limit, with `actual` greater than the limit. Its `actual` value
must equal the report summary's net growth. Required-path checks inspect the
complete candidate manifest, including entries ignored in the comparison.

Warnings contain `source`, `path`, `code` and `key`. The code is
`uncompared_metadata`; the key must be nonempty valid UTF-8. Metadata values are
never recorded. Warning paths are canonical, except that `.` identifies
metadata on an ignored root directory record. Such warnings remain even though
there is no root manifest entry. The path `.` is invalid for entries, hardlink
targets, ignores and required paths.

In a manifest, a warning's `source` is empty and each nonroot warning refers to
an explicitly recorded entry. In a diff or report, `source` is `before` or
`after`, including for ignored paths. Output warnings are unique and sorted by
path, code, key and source, using Go string order. When only the source differs,
`after` comes before `before`. A warning identifies metadata that was not
compared; it does not establish an effect on a running system.

`metadata_exclusions` is the fixed sorted list returned as a fresh slice by
`MetadataExclusions()`: `acls`, `directory_sizes`, `file_capabilities`,
`selinux_labels`, `timestamps`, `unrecognized_pax_metadata`, `user_group_names`,
`xattrs`. Every report includes this list. Entry order, compression and padding
do not affect the comparison. Hardlink header metadata is ignored; hardlinks use
the effective metadata of the regular file at the end of their link chain.

Changing the schema version requires coordinated updates to models, examples
and consumers. These examples define the version 1 output fields and allowed
values. Text and Markdown renderers must escape control characters in strings
taken from archives. Markdown also escapes Markdown and HTML syntax.

## Executable fixtures

- [Complete report](../testdata/models/report-v1.json): three entries on each
  side, with one addition, one removal, one modification and one unchanged entry.
  Payload grows from 8 to 9 bytes, for net growth of +1 byte.
  `etc/device.conf` changes from mode `0640` (JSON 416) to `06750` (JSON 3560),
  size 2 to 4 and a changed SHA-256. `etc/new` adds 2 bytes and `obsolete` removes
  3. The two +2-byte growth records are sorted by path to break the tie. A
  zero-byte growth limit is exceeded, and required path `usr/bin/agent` is missing.
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

Expected fixture values were written independently of the production scanner
and comparator. Tests decode, validate, encode and decode reports again. They
check exact fields and ordering, along with the expected modes, changed fields
and growth values.

## Errors

Use `errors.Is` with `ErrInvalidInput`, `ErrUnsupportedInput` or
`ErrLimitExceeded` to identify input error categories. Use `errors.As` to inspect
details: `*Error` records the operation, archive path, category and underlying
error; `*LimitError` records the limit name, maximum, first observed value that
exceeded it, and optional underlying error. An empty `Error.Kind` adds context
without assigning an input category. Standard I/O errors and cancellation can
still be reached through `Unwrap`. Model validation failures are categorized as
invalid input, even when no archive parser has run.
