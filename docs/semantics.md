# Archive and comparison semantics

This is a user-oriented reference to the implemented [v0.1 contract](v0.1-contract.md) and [models](models.md).

## Inputs and entry kinds

v0.1 reads uncompressed tar or gzip-compressed tar, detecting gzip from magic bytes. Ordinary USTAR, GNU and per-file PAX handling use Go's `archive/tar`. Supported entries are regular files, directories, symlinks, hardlinks, character devices, block devices and FIFOs. Device records describe metadata only. Contents are never extracted, executed or used to follow a host link, and archive URLs are never fetched.

Empty tar archives and EOF after a complete entry follow Go tar semantics; a custom two-zero-block terminator is not required. Truncated headers or payloads fail. After tar EOF, only zero decoded padding is accepted. Gzip is read through EOF to validate its checksum. Concatenated gzip members are accepted only when the combined decoded bytes are one valid tar stream plus zero padding. Another tar archive or nonzero trailing bytes fail.

Global PAX headers, sparse archives (GNU type and GNU/SCHILY sparse metadata), unknown entry kinds and payload bytes on non-regular entries are unsupported. Raw firmware, ext4, SquashFS, cpio, OCI layers, archive URLs, standard input and extracted-directory scanning are outside v0.1.

## Paths and explicit entries

Entry paths use POSIX root-relative names. `./etc//device.conf` normalizes to `etc/device.conf`. Absolute paths, any `..` segment, invalid UTF-8 and duplicate normalized paths fail. A backslash is a literal filename character, including on macOS. Case is preserved. No host path resolution is involved.

A root `.` directory record is ignored; a non-directory root record fails. Parents may be implicit, but an explicitly recorded ancestor must be a directory. Manifests retain explicit entries only. For example, an archive containing `etc/device.conf` without an `etc` entry has one entry, and adding a later explicit `etc` directory is a reported addition. No parent metadata is invented.

Caller-built model paths, ignore rules and required policy paths must already be canonical. They are validated, not normalized. They contain no leading/trailing slash, repeated separator, `.` or `..` segment. Glob characters have no special meaning. The warning-only path `.` identifies metadata on the ignored root directory and is invalid for entries, ignores and required paths.

## Compared fields

| Field | Meaning |
|---|---|
| `kind` | One of the seven supported entry kinds |
| `mode` | Permission, setuid, setgid and sticky bits masked to `07777` |
| `uid`, `gid` | Numeric tar IDs, preserving signed values without host name lookup |
| `size`, `sha256` | Streamed regular payload bytes and SHA-256; also effective hardlink data |
| `link_target` | Verbatim symlink target or canonical hardlink target |
| `device_major`, `device_minor` | Device numbers for character/block records |

JSON modes are decimal integers: `0644` is `420`, and `04755` is `2541`. Human reports use octal. File content is hashed in bounded reads; payloads are not retained. Memory scales with recorded entries and metadata rather than total regular-file payload bytes. Size, counter and total arithmetic uses checked 64-bit integers.

Changes sort lexicographically by canonical path. A kind transition is one modified record with both sides. Modified records include every changed field in the table's order, with `size` before `sha256` and device major before minor. Added/removed records carry the complete applicable side. Timestamp-only differences do not produce changes.

## Links

Symlink targets are preserved verbatim as valid UTF-8. Absolute targets, `..` and empty targets are allowed. No symlink is followed, and dangling targets are not inferred.

Hardlink targets are normalized using entry-path rules. Forward references and chains are allowed if they terminate at a regular entry. Missing targets, cycles and non-regular endpoints fail. Each alias keeps its own path, kind and canonical target, but inherits effective size, digest, mode, UID and GID from the terminal regular entry. Its header's ownership/mode do not describe a separate inode and are ignored.

Changing terminal regular content appears at each hardlinked path. Changing an alias target is separately visible. Swapping which name stores the regular payload versus a hardlink can change topology and the positive-growth list while leaving net payload growth zero; inode canonicalization is outside scope.

## Exclusions and warnings

Every report declares `acls`, `directory_sizes`, `file_capabilities`, `selinux_labels`, `timestamps`, `unrecognized_pax_metadata`, `user_group_names` and `xattrs` as exclusions. Entry order, compression and archive padding are representation details outside comparison. A no-change report covers the compared data only.

Encountered xattrs, ACLs, capabilities, SELinux labels and unrecognized vendor PAX keys produce `uncompared_metadata` warnings. A warning records path and key, never the metadata value. Ignored paths retain warnings. Root-directory metadata warnings use path `.` even though no root entry is kept. Invalid UTF-8 metadata keys fail rather than being replaced silently.

Standard PAX path/linkpath/size/uid/gid fields are handled as entry data. Deliberately ignored time/name fields do not produce warnings. Sparse metadata is an error. A warning identifies an unexamined field and does not establish any runtime consequence.

## Growth accounting

Totals sum retained regular entries after applying the same ignores to both sides. Hardlink aliases, symlinks, directories, devices and FIFOs contribute zero. Net growth is candidate payload total minus baseline payload total, including additions and removals; it can be negative.

This measures uncompressed regular payload bytes. It does not measure compressed archive size, filesystem block allocation, flash partition size or OTA transfer size. Metadata-only changes may have zero growth.

Positive-growth records include added regular payloads, positive regular-to-regular size deltas and transitions from another kind to regular. A transition away from regular removes its old payload from the total. Lists sort by descending growth bytes, then path. JSON includes the full list; human reports show at most ten growth records and all changes. A policy compares the net total, so removals/shrinkage can offset additions. See [ignore and policy interactions](cli.md).

## Resource limits

| Go option | Default | Budget |
|---|---|---|
| `MaxEntries` | `100000` | Logical entries per archive, including ignored root-directory records |
| `MaxArchiveBytes` | `8589934592` (8 GiB) | Decoded bytes including headers, metadata and trailing padding |
| `MaxFileBytes` | `2147483648` (2 GiB) | Each regular-file payload |
| `MaxPathBytes` | `4096` | Each decoded entry path or link target before normalization, in UTF-8 bytes |

Zero Go option fields select defaults. Positive values override them; negative values fail. CLI overrides must be positive decimal numbers. The archive budget covers reads needed to validate EOF/checksums, so a mostly empty or highly compressed input can still exceed it. Path/target caps apply after decoding archive metadata; `archive/tar` supplies its bounded metadata extension handling. Limits are input resource budgets, not byte-for-byte host memory limits.

Scan failures return no successful partial manifest. Cancellation is checked between bounded reads; it cannot interrupt an arbitrary reader already blocked inside `Read`. Callers should provide a reader with its own cancellation/deadline behavior when needed. See [library ownership and errors](library.md).
