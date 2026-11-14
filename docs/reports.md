# Reports and schema compatibility

Text, JSON and Markdown present the same validated report data. See the [model fields and invariants](models.md), [JSON schema](report-v1.schema.json) and [complete JSON fixture](../testdata/models/report-v1.json).

## Formats

Text and Markdown summarize counts, retained payload totals, net growth, changes, violations and warnings. All changes are rendered; the positive-growth ranking displays at most ten entries. Human mode values use octal. Paths/targets and other untrusted text are escaped for control characters and terminal escapes, plus Markdown metacharacters and HTML where appropriate. Escaped spelling represents filename data rather than a command to execute.

JSON retains every change and positive-growth record. Modes are decimal integers representing the `07777` mask. Numeric sizes, counts, IDs and device numbers are integers; use an integer-capable consumer for full 64-bit precision. A required-path violation has null `limit`/`actual`; a growth violation uses an empty `path` and numeric `limit`/`actual`.

JSON uses two-space indentation and a final newline. JSON strings escape HTML-sensitive characters and nonprinting controls. Human strings are quoted with visible Go escapes; Markdown places them inside code elements with punctuation encoded as entities.

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

Empty arrays are `[]`, not null. Absent change sides are explicit null, never omitted. An added/removed record has an empty `changed_fields` array. Every nested entry carries `path`, `kind`, `mode`, `uid`, `gid`, `size`, `sha256`, `link_target`, `device_major` and `device_minor`; fields that do not apply are zero or empty strings. Regular/hardlink digests are lowercase SHA-256 hex strings.

Summary entry counts describe both sides after ignores. Before count equals removed + modified + unchanged, and after count equals added + modified + unchanged. Payload totals include retained regular entries only. Required-path checks use the complete candidate, so violations can refer to a path outside retained changes. `--fail-on-change` is a CLI gate and does not create an invented policy-violation rule.

## Determinism and validation

For the same logical inputs/options, reports must be byte-identical. No timestamps, absolute host filenames, random identifiers or run-dependent values belong in output. Entry order and gzip representation do not affect comparison.

Changes sort by canonical path. Modified field order is `kind`, `mode`, `uid`, `gid`, `size`, `sha256`, `link_target`, `device_major`, `device_minor`. Growth sorts by descending bytes then path. Violations sort by rule then path. Warnings sort uniquely by path, code, key, source; on a final source tie, `after` precedes `before`. Warnings survive ignores; `.` is allowed only as a warning path for root-directory metadata.

The schema describes structure. Go `Validate` adds semantic checks for canonical paths, field applicability, ordering, uniqueness, arithmetic and cross-field consistency. Ordinary JSON decoding and successful marshaling do not replace validation. `report.Write` validates the report before writing. A downstream I/O failure may still leave a prefix of the output; the CLI returns 2 and the library returns the writer error.

Treat `schema_version` as part of the consumer contract and reject unsupported versions. During 0.x development, pin a module commit and review API/schema changes explicitly. Human text formatting is for review; use the versioned JSON fields for automation.
