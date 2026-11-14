# Public report renderer

Import `github.com/Azhovan/rootfs-diff/report` and call
`report.Write(writer, model, report.Text)`. `report.JSON` and `report.Markdown`
select the other formats. The executable `ExampleWrite` demonstrates constructing
a report from comparison results. Scanning and policy evaluation remain in the
root package; the renderer only presents validated model data.

`Write` validates the complete report before calling the writer. It rejects
unknown formats with `rootfsdiff.ErrUnsupportedInput`; invalid reports and a nil
writer return `rootfsdiff.ErrInvalidInput`. Use `errors.Is` or `errors.As` through
the returned error. A zero-value format is unsupported. Supply schema version 1
and the exact list from `rootfsdiff.MetadataExclusions()` even for an empty report.

JSON follows [schema version 1](../docs/report-v1.schema.json), with all fields in
[model order](../docs/models.md), two-space indentation and a final newline.
Version changes require coordinated model/schema/consumer changes. Empty slices
are `[]`; absent change sides and inapplicable violation numbers are explicit
`null`. All integers are exact decimal JSON numbers, including signed UID/GID,
64-bit counts and bytes, and mode bits as a decimal integer in the `07777` mask.
Consumers must preserve 64-bit integer precision. JSON strings use Go's standard
JSON escaping, including HTML-sensitive characters, plus Unicode escapes for
DEL, C1 and nonprinting Unicode characters. It has no host source
filenames, timestamps or run-dependent values.

All formats include the summary, every complete change, positive growth,
violations, source-tagged warnings and the fixed metadata exclusions. Changes
sort by canonical path; changed fields use the fixed model order. Growth sorts
by descending bytes then path; violations by rule then path; warnings by path,
code, key, source (so `after` precedes `before` on a tie). Input ordering is
validated, never silently sorted or mutated. Equivalent valid models, including
nil versus empty slices, produce identical bytes.

Text and Markdown include counts, regular-payload totals and signed net growth,
every applicable before/after entry field, and at most ten growth rankings. The
ranking heading explicitly shows the displayed and total counts; JSON retains
the entire ranking. Human modes use octal with a leading zero, preserving all
special bits. Hardlink effective sizes/digests are shown, but aliases do not
contribute payload totals. Added/removed sides display `absent`.

Every untrusted human-facing string is double-quoted with Go string escapes.
Newlines, tabs, carriage returns, terminal escape sequences, C1 controls and
Unicode format controls are visible escapes. Printable Unicode remains readable;
long strings and changes are never truncated. Markdown wraps these quoted strings
in HTML `code` elements with ASCII punctuation encoded as numeric entities, so
Markdown syntax, links and HTML remain literal when rendered. Metadata warning
keys are shown, never their values. No changes or lack of violations establishes
firmware equivalence, bootability, compatibility or security; reports always state
the excluded metadata.

For example, an added symlink appears in text as:

```text
  added "bin/current"
    Before: absent
    After: kind=symlink mode=0777 uid=0 gid=0 link_target="../lib/agent"
```

Each report is encoded in memory, then written once. A writer error may leave a
partial output; do not consume that output as an intact report. Underlying errors
remain reachable; a short write with no error returns `io.ErrShortWrite`. There
are no retries, process exits or implicit stdout writes. `Write` does not close
or flush the caller-owned writer. Independent calls share no mutable state; the
caller must avoid concurrent mutation of the model or unsafe writer sharing.
