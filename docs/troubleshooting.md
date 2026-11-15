# Troubleshooting

Use the sections below to diagnose common problems in v0.1. If you use the Go library, inspect the [typed errors](library.md#typed-errors) for details. If you use the command-line tool, check its [exit code](cli.md#output-and-exit-codes) before using the report.

## Private access

The upstream repository is public and requires neither GitHub credentials nor a `GOPRIVATE` setting. If a private copy or fork produces a repository-not-found, authentication or module-proxy error, first check that your Git credentials have read access and that the module path points to the intended repository.

If you are signed in through the GitHub CLI, run `gh auth setup-git` to configure HTTPS Git access. An SSH login does not configure HTTPS module downloads; configure credentials for the transport your Git setup uses.

Set `GOPRIVATE` before fetching a private module for the first time, preserving other configured patterns. If you have set `GONOPROXY` or `GONOSUMDB` separately, make sure those settings also cover the private module. An unexpected public proxy or checksum lookup for a private path usually means the settings do not match. Those lookups are normal for the public upstream module. `GOPRIVATE` controls how Go fetches modules; it does not supply credentials. See [installation](installation.md).

For offline checks, install a Go toolchain that satisfies `go.mod` before setting `GOTOOLCHAIN=local` or disconnecting. Use Go 1.27.4 to match the repository. A toolchain download error occurs before archive scanning begins.

## Unsupported or corrupt input

Supply complete tar or tar.gz root filesystem archives. Renaming an ext4, SquashFS, cpio, OCI, ZIP or raw firmware image to end in `.tar` does not make it a supported input. The tool detects gzip from the file's bytes. The CLI accepts two local regular files, with all flags before the filenames.

A scan fails on unsupported entry kinds, global PAX headers, GNU/SCHILY sparse data, or payload bytes attached to anything other than a regular file. Malformed headers, truncated file contents, gzip checksum errors and nonzero trailing decoded bytes also cause failure. Recreate or recopy the archive from a trustworthy build output and try again. A failed scan provides no usable partial result. Removing unsupported metadata changes the archive, so first understand what would be lost.

## Invalid paths and hardlinks

Archive entry paths must be valid UTF-8 and must not be absolute or contain `..` segments. Two entries cannot normalize to the same path: for example, `etc/device.conf` and `./etc//device.conf` cannot both appear. If the archive records an ancestor of another entry, that ancestor must be a directory. For example, a regular file named `etc` cannot also have a child named `etc/device.conf`. Correct these paths when creating the archive.

Paths in ignore rules and policies must already be canonical, with no leading slash, `.` or `..` segments, repeated separators or trailing slash.

Every hardlink chain must end at a recorded regular file. Missing targets, cycles and chains ending at directories or symlinks cause failure. Symlink targets follow a different rule: they are preserved as written and never followed. An absolute symlink target is therefore allowed, even though an absolute entry path is invalid.

## Limits

A limit error identifies which limit was exceeded. In Go, inspect `*rootfsdiff.LimitError` for details. Its `Actual` field is the first observed value above the limit; the final total could be larger if input remains unread. Check how the archive was produced before choosing a larger positive limit. A failed scan does not return a complete manifest.

`--max-archive-bytes` counts decoded headers, metadata extensions, padding and file contents. It does not measure the compressed file's size. `--max-path-bytes` counts UTF-8 bytes, rather than characters, for each decoded entry path or link target separately. Zero selects the default in Go options but is invalid as a command-line override. See [resource limits](semantics.md#resource-limits).

## Metadata warnings and unexpected changes

An `uncompared_metadata` warning identifies a metadata key whose value is not compared. It does not fail a policy check. Even when there are no warnings, the standard exclusions listed in the report still apply. A warning path of `.` refers to metadata on a root directory record that the scanner otherwise ignores. Comparison ignore rules do not hide warnings.

If file contents appear unchanged, check numeric UID/GID and all permission bits, including the special bits in the `07777` mask. Hardlinks inherit the content and metadata of the regular file at the end of their chain, so changing one file can affect several reported paths. Adding or removing a directory record is also a change, even if its children are unchanged. Timestamp or compression changes alone produce no difference.

Reported growth measures uncompressed regular-file contents. It does not measure disk usage, flash usage, compressed archive size or over-the-air transfer size.

## Required paths and gate failures

A required path must be explicitly recorded in the candidate archive, using the canonical spelling given in the policy. A parent directory implied by a child's path does not count unless the archive records it separately. A symlink counts as present without proving that its target exists or can run. Ignore rules do not bypass this check. Check the path's exact spelling and the entries in the candidate archive.

Differences alone return exit code 0. Code 1 means an enabled check failed, and the comparison still produced a complete report. A policy can require paths or limit net growth; `--fail-on-change` fails on any change left after ignores. Growth equal to the limit passes, as does a reduction in total size. Look in `violations` for failed policy checks and in `changes` for the changes that caused `--fail-on-change` to fail.

## Output and writer errors

If parsing, scanning or comparison fails, the command writes no report bytes. Shell redirection may still create an empty output file. A failure while writing, such as a full destination filesystem or a downstream reader that closes early, can leave a partial report and returns exit code 2. Check the exit code before parsing or saving the output as a completed report.

In Go, check the error returned by `report.Write` and any relevant error when closing the output file. Wrapped I/O errors remain available for inspection. In CI, use the [exit-status example](ci.md) so a later successful command or `tee` does not hide a failed comparison.
