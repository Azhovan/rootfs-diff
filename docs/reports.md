# Reports and schema compatibility

Text, JSON and Markdown reports present the same validated data. This guide
explains how to read and consume them. See the [model fields and validation
rules](models.md), [JSON schema](report-v1.schema.json) and [complete JSON
example](../testdata/models/report-v1.json) for the exact structure.

## Formats

Text and Markdown show entry counts, payload totals after ignores, net growth,
changes, policy violations and warnings. They include every change and up to ten
entries in the positive-growth ranking. File modes are shown in octal.

Paths, link targets and other text taken from archives are quoted and escaped so
that control characters and terminal escape sequences are visible. Markdown
reports also escape Markdown and HTML syntax. For example, an escaped newline is
part of the displayed filename, rather than a line break in the report.

JSON includes every change and positive-growth record. Modes are decimal
integers containing the permission and special bits in the `07777` mask. Sizes,
counts, IDs and device numbers are integers; consumers must preserve their full
64-bit precision. In a required-path violation, `limit` and `actual` are `null`.
In a growth violation, `path` is empty and `limit` and `actual` contain byte counts.

JSON uses two-space indentation and a final newline. Its strings escape
HTML-sensitive characters and nonprinting controls. Text and Markdown use
quoted strings with visible Go escape sequences. Markdown places those strings
inside HTML code elements and encodes punctuation as entities.

## JSON layout

| Top-level field | Contents |
|---|---|
| `schema_version` | Integer `1` |
| `summary` | Before/after entry counts, added/removed/modified/unchanged counts, before/after payload bytes and signed net growth |
| `changes` | Path, change kind, complete applicable before/after entries and all changed fields |
| `positive_growth` | Every positive regular payload contribution as `path`/`bytes` |
| `violations` | Policy rule, path, limit and actual |
| `warnings` | Source (`before`/`after`), path, code and metadata key; no metadata values |
| `metadata_exclusions` | Complete standard exclusion list, even with no changes or warnings |

Empty arrays are written as `[]`. Both `before` and `after` are always present;
the missing side of an addition or removal is `null`. Added and removed records
have an empty `changed_fields` array. Every nested entry includes `path`, `kind`,
`mode`, `uid`, `gid`, `size`, `sha256`, `link_target`, `device_major` and
`device_minor`. Fields that do not apply contain zero or an empty string.
Regular-file and hardlink digests are lowercase SHA-256 hexadecimal strings.

Summary counts describe the entries left after ignore rules are applied. The
baseline count equals removed + modified + unchanged; the candidate count equals
added + modified + unchanged. Payload totals count only retained regular files.
Required-path checks inspect the complete candidate, so a violation may refer
to a path excluded from the comparison. The CLI's `--fail-on-change` check can
cause exit 1, but does not add a policy violation to the report.

## Determinism and validation

The same logical archive contents and options produce byte-identical reports.
Reports contain no timestamps, absolute host filenames, random identifiers or
other values that vary between runs. Entry order and gzip encoding do not
affect the comparison.

Changes are sorted by canonical path. Modified fields appear in this order:
`kind`, `mode`, `uid`, `gid`, `size`, `sha256`, `link_target`, `device_major`,
`device_minor`. Growth records are sorted by descending bytes, then path;
violations by rule, then path. Warnings are unique and sorted by path, code, key
and source, in that order. When only the source differs, `after` comes before
`before`. Ignore rules do not hide warnings. Only a warning about root-directory
metadata may use `.` as its path.

The JSON schema checks the report's structure. The Go `Validate` methods also
check path rules, which fields apply to each entry kind, ordering, uniqueness,
arithmetic and consistency between fields. Decoding or encoding JSON
successfully does not perform those additional checks. `report.Write` validates
the report before writing it. If the output writer later fails, the output may
be partial; the CLI returns 2 and the library returns the writer error.

Consumers should check `schema_version` and reject versions they do not support.
During 0.x development, pin a module commit and review API or schema changes
before upgrading. Use the versioned JSON fields for automation; text and
Markdown are intended for human review.
