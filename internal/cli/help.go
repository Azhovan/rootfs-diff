package cli

const helpText = `Usage: rootfs-diff [flags] BEFORE.tar[.gz] AFTER.tar[.gz]

Compare two local regular files containing tar or gzip-compressed tar archives.
Gzip is detected by magic bytes. Archives are streamed without extraction or
execution. Stdin, URLs, extracted directories and other formats are unsupported.
Input symlinks are unsupported. Flags must precede both paths. Use ./ for local
filenames beginning with '-'.

Options:
  --format text|json|markdown  Report format (default text).
  --policy FILE               Strict schema-version-1 JSON policy (optional).
  --ignore-path PATH          Ignore an exact canonical archive path; repeatable.
  --ignore-prefix PATH        Ignore a path and its descendants; repeatable.
  --fail-on-change            Exit 1 for any non-ignored change.
  --max-entries N             Logical entry limit (default 100000).
  --max-archive-bytes N       Decoded bytes including padding (default 8589934592).
  --max-file-bytes N          Per-regular-file payload bytes (default 2147483648).
  --max-path-bytes N          Path or link target bytes (default 4096).
  --help                      Show this help without opening inputs.
  --version                   Show version, or "dev" for ordinary builds.

Limits are positive decimal int64 values; explicit zero is invalid. Ignore rules
are literal canonical POSIX root-relative paths, with no glob/regex expansion.
Prefix var/log matches var/log and var/log/..., but never var/logger. Warnings
remain visible for ignored paths. Required-path policy checks use the complete
candidate manifest and count only explicit entries. Growth checks use net regular
payload growth after ignores; hardlink aliases do not add payload bytes.

Exit status:
  0  Comparison succeeded and enabled checks passed. Ordinary differences pass.
  1  Policy violation or --fail-on-change gate. A complete report is still emitted.
  2  Invalid arguments/policy/input, unsupported input, limits, I/O, rendering or
     cancellation failure. Pre-render errors emit no report. Downstream write
     failures can leave partial output; always check the exit status.

Stdout contains only the report (or requested help/version); diagnostics use
stderr. Redirect stdout to save a report. Reports exclude timestamps, user/group
names, xattrs, ACLs, capabilities, SELinux labels and other declared metadata. No
changes does not establish firmware equivalence, bootability or security.

Examples:
  rootfs-diff --format json before.tar after.tar.gz > report.json
  rootfs-diff --policy policy.json --ignore-prefix var/log before.tar after.tar
  rootfs-diff --fail-on-change before.tar after.tar
`
