# Archive and comparison semantics

This guide explains what rootfs-diff reads, compares and reports. For the exact
API requirements, see the [v0.1 contract](v0.1-contract.md) and [models](models.md).

## Inputs and entry kinds

v0.1 reads uncompressed tar and gzip-compressed tar archives. It recognizes gzip
from the bytes at the start of the input, regardless of the filename. Go's
`archive/tar` package handles ordinary USTAR, GNU and per-file PAX records.

Supported entries are regular files, directories, symbolic links (symlinks),
hardlinks, character devices, block devices and named pipes (FIFOs). Device
entries describe metadata only. rootfs-diff reads archive entries without
extracting or executing them, following links on the host, or fetching URLs.

The scanner accepts an empty tar archive and a stream that ends immediately
after a complete entry, following Go's tar reader. It does not require two
zero-filled terminator blocks. Truncated headers or file contents cause an error.
After the tar stream ends, any remaining decoded bytes must be zero padding.
The scanner reads gzip input to the end to verify its checksum. Multiple gzip
members are accepted only if their combined decoded contents form one valid tar
stream followed by zero padding. A second tar archive or nonzero trailing data
causes an error.

The scanner rejects global PAX headers, sparse archives (including GNU sparse
entries and GNU/SCHILY sparse metadata), unknown entry kinds, and payload bytes
on non-regular entries. v0.1 does not accept raw firmware, ext4, SquashFS, cpio,
OCI layers or extracted directories. The CLI requires local regular files and
does not accept URLs or standard input. The library's `Scan` function reads a
caller-supplied `io.Reader`; it does not open files or fetch URLs itself.

## Paths and explicit entries

Paths are relative to the root of the archived filesystem and use POSIX `/`
separators. The scanner removes repeated separators and `.` segments, so
`./etc//device.conf` becomes `etc/device.conf`. This normalized form is called a
canonical path. Absolute paths, any `..` segment, invalid UTF-8 and duplicate
canonical paths cause an error. A backslash is a literal filename character,
including on macOS. Paths retain their original case and are never resolved
against the host filesystem.

The scanner ignores a root `.` directory record and rejects a non-directory
record at the root. A parent directory may be absent from the archive, but if it
is explicitly recorded, it must be a directory. The manifest, which is the list
of scanned entries and their metadata, contains only explicitly recorded entries.
For example, an archive containing `etc/device.conf` without an `etc` entry has
one manifest entry. Adding an explicit `etc` directory record in the candidate
is reported as an addition, even if `etc/device.conf` is unchanged. The scanner
does not invent entries or metadata for missing parents.

Paths in caller-built models, ignore rules and required-path policies must
already be canonical. Validation rejects a leading or trailing slash, repeated
separators, and `.` or `..` segments rather than normalizing them. Glob characters
have no special meaning. Only warnings may use `.` as a path, to identify
metadata on the ignored root directory record.

## Compared fields

| Field | Meaning |
|---|---|
| `kind` | One of the seven supported entry kinds |
| `mode` | Permission, setuid, setgid and sticky bits masked to `07777` |
| `uid`, `gid` | Numeric tar IDs, preserving signed values without host name lookup |
| `size`, `sha256` | Size in bytes and SHA-256 of regular-file contents; hardlinks use the values of the regular file they reference |
| `link_target` | Verbatim symlink target or canonical hardlink target |
| `device_major`, `device_minor` | Device numbers for character/block records |

JSON modes are decimal integers: `0644` is `420`, and `04755` is `2541`. Text
and Markdown reports use octal. The scanner hashes file contents in bounded
chunks and retains only metadata and hashes. Its memory use grows with the
number of entries and their metadata, rather than the total size of file
contents. Arithmetic on sizes, counters and totals uses 64-bit integers with
overflow checks.

Changes are sorted lexicographically by canonical path. If an entry changes
kind, it appears as one modification with both its before and after values.
Modified records list every changed field in the table's order, with `size`
before `sha256` and device major before minor. Added records contain the complete
candidate entry; removed records contain the complete baseline entry. A
difference in timestamps alone is not reported as a change.

## Links

Symlink targets must be valid UTF-8 and are preserved exactly as recorded.
Absolute targets, `..` and empty targets are allowed. The scanner does not follow
symlinks or determine whether their targets exist.

Hardlink targets follow the same normalization rules as entry paths. A hardlink
may reference an entry later in the archive or another hardlink, provided the
chain ends at a regular file. Missing targets, cycles and chains ending at a
non-regular entry cause an error. Each hardlink keeps its own path, kind and
canonical target. Its effective size, digest, mode, UID and GID come from the
regular file at the end of the chain. The hardlink header's metadata is ignored
because a hardlink does not describe a separate inode.

A content change in the regular file appears at every hardlinked path. A change
to a hardlink's target is reported separately. If an archive swaps which path
holds the regular-file payload and which is a hardlink, the link structure and
positive-growth list can change while net growth stays zero. rootfs-diff does
not rewrite hardlink groups to choose a common representative path.

## Exclusions and warnings

Every report lists the metadata that is excluded from comparison: `acls`,
`directory_sizes`, `file_capabilities`, `selinux_labels`, `timestamps`,
`unrecognized_pax_metadata`, `user_group_names` and `xattrs`. Entry order,
compression and archive padding also do not affect the comparison. A report
with no changes means that the compared data matches; it does not establish
equivalence for the excluded metadata.

When the scanner encounters extended attributes (xattrs), access control lists
(ACLs), file capabilities, SELinux labels or unrecognized vendor PAX keys, it
emits an `uncompared_metadata` warning. Each warning identifies the path and
metadata key, without recording the value. Ignore rules do not hide warnings.
Warnings about metadata on the root directory use path `.`, even though the
manifest has no root entry. Invalid UTF-8 metadata keys cause an error rather
than being silently replaced in the report.

Standard PAX fields `path`, `linkpath`, `size`, `uid` and `gid` are handled as
entry data. The time and name fields that are deliberately ignored do not
produce warnings. Sparse metadata causes an error. A warning means that a field
was not compared; it does not establish how that field affects a running system.

## Growth accounting

Payload totals sum the sizes of regular files that remain after applying the
same ignore rules to the baseline and candidate. Hardlink aliases, symlinks,
directories, devices and FIFOs contribute zero. Net growth is the candidate
total minus the baseline total, including additions and removals. It is negative
when the candidate is smaller.

This measures the uncompressed contents of regular files. It does not measure
compressed archive size, filesystem block allocation, flash partition size or
OTA transfer size. Metadata-only changes may have zero growth.

The positive-growth list includes new regular files, size increases in existing
regular files, and entries that change from another kind to a regular file.
Changing a regular file to another kind removes its old payload from the total.
Growth records are sorted from largest increase to smallest, with ties sorted
by path. JSON includes the full list; text and Markdown show at most ten growth
records and all changes. A growth policy checks the net total, so removals and
size reductions can offset additions. See [ignore and policy interactions](cli.md).

## Resource limits

| Go option | Default | Budget |
|---|---|---|
| `MaxEntries` | `100000` | Logical entries per archive, including ignored root-directory records |
| `MaxArchiveBytes` | `8589934592` (8 GiB) | Decoded bytes including headers, metadata and trailing padding |
| `MaxFileBytes` | `2147483648` (2 GiB) | Each regular-file payload |
| `MaxPathBytes` | `4096` | Each decoded entry path or link target before normalization, in UTF-8 bytes |

In Go scan options, zero selects the default, a positive value overrides it,
and a negative value causes an error. CLI overrides must be positive decimal
numbers. The archive-byte limit includes reads needed to reach the end and
validate checksums, so a mostly empty or highly compressed input can still
exceed it. Path and target limits apply after decoding archive metadata and
before normalization. Go's `archive/tar` reader also bounds metadata extension
handling. These limits constrain input processing; they do not specify an exact
limit on host memory use.

Scan failures return no successful partial manifest. The scanner checks
cancellation between bounded reads, but cannot interrupt an arbitrary reader
already blocked inside `Read`. When that matters, callers should provide a
reader with its own cancellation or deadline support. See [library ownership
and errors](library.md).
