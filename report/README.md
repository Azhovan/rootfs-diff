# Public report renderer

To write a report, import `github.com/Azhovan/rootfs-diff/report` and call
`report.Write(writer, model, report.Text)`. `report.JSON` and `report.Markdown`
select the other formats. The executable `ExampleWrite` shows how to construct
a report from comparison results. The root package handles scanning and policy
evaluation; the renderer presents the resulting validated data.

`Write` validates the complete report before calling the writer. It rejects
unknown formats with `rootfsdiff.ErrUnsupportedInput`. Invalid reports and a nil
writer return `rootfsdiff.ErrInvalidInput`. Use `errors.Is` to check the error
category or `errors.As` to inspect its details. A zero-value format is
unsupported. Supply schema version 1
and the exact list from `rootfsdiff.MetadataExclusions()` even for an empty report.

JSON follows [schema version 1](../docs/report-v1.schema.json), with all fields in
[model order](../docs/models.md), two-space indentation and a final newline.
Version changes require coordinated updates to models, the schema and consumers.
Empty slices are `[]`; absent change sides and inapplicable violation numbers
are explicitly `null`. Integers are exact decimal JSON numbers. This includes
signed UID/GID, 64-bit counts and byte sizes, and mode bits in the `07777` mask.
Consumers must preserve 64-bit integer precision.

JSON strings use Go's standard escaping, including for HTML-sensitive characters.
They also use Unicode escapes for the DEL character, C1 control characters and
nonprinting Unicode characters. Reports contain no host source filenames,
timestamps or values that vary between runs.

Every format includes the summary, all complete changes, positive growth,
violations, warnings identifying the source archive, and the fixed metadata
exclusions. Changes are sorted by canonical path, and changed fields use the
fixed model order. Growth records are sorted by descending bytes, then path;
violations by rule, then path; and warnings by path, code, key and source. When
only the warning source differs, `after` comes before `before`. The renderer
validates input ordering rather than silently sorting or changing the input.
Equivalent valid models, including nil and empty slices, produce identical bytes.

Text and Markdown include entry counts, regular-file payload totals, signed net
growth and every applicable before/after entry field. They show at most ten
entries in the growth ranking, with a heading giving both the displayed and
total counts. JSON includes the whole ranking. File modes use octal with a
leading zero, preserving special bits. Hardlink sizes and digests are shown
using their effective values, but aliases do not count again in payload totals.
The missing side of an addition or removal is displayed as `absent`.

Every string taken from an archive is double-quoted with Go string escapes in
text and Markdown.
Newlines, tabs, carriage returns, terminal escape sequences, C1 controls and
Unicode format controls are visible escapes. Printable Unicode remains readable;
long strings and changes are never truncated. Markdown wraps these quoted strings
in HTML `code` elements with ASCII punctuation encoded as numeric entities, so
Markdown syntax, links and HTML remain literal when rendered. Metadata warning
keys are shown, without their values. A report with no changes or violations
does not establish firmware equivalence, bootability, compatibility or security.
Reports always list the excluded metadata.

For example, an added symlink appears in text as:

```text
  added "bin/current"
    Before: absent
    After: kind=symlink mode=0777 uid=0 gid=0 link_target="../lib/agent"
```

Each report is encoded in memory, then written in one call. A writer error may
leave partial output, which must not be treated as a complete report. Returned
errors preserve the underlying cause; a short write with no error returns
`io.ErrShortWrite`. `Write` does not retry, exit the process, write to stdout
implicitly, close the writer or flush it. The caller owns the writer.
Independent calls share no mutable state. Callers must not modify the model
while it is being rendered, and must coordinate access to shared writers.
