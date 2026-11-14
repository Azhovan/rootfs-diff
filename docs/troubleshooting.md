# Troubleshooting

These diagnostics explain v0.1 behavior. Library callers can inspect [typed error categories](library.md#typed-errors); CLI callers should always check exits 0, 1 and 2.

## Private access

This upstream repository is public; it needs no GitHub credentials or `GOPRIVATE` pattern. For repository-not-found, authentication or module-proxy errors with a private copy/fork, first verify the Git credential setup has read access to that repository and the requested module path points to it. An authenticated GitHub CLI session can configure HTTPS Git access with `gh auth setup-git`. An SSH login alone does not configure HTTPS module downloads; use the Git transport/credential configuration that works for your environment.

For the private module, set `GOPRIVATE` before the initial lookup and preserve other configured patterns. If `GONOPROXY`/`GONOSUMDB` are independently set, make them consistent with your private-module routing. An unexpected public proxy or checksum lookup for a private path usually means those settings do not match. Public upstream proxy/checksum lookups are normal. `GOPRIVATE` never supplies credentials. See [installation](installation.md).

Offline checks also need a locally installed Go toolchain satisfying `go.mod`. Install pinned Go 1.27.4 before setting `GOTOOLCHAIN=local` or disconnecting. A toolchain download failure is not an archive error.

## Unsupported or corrupt input

Use complete tar or tar.gz rootfs archives. A `.tar` extension does not turn an ext4, SquashFS, cpio, OCI, ZIP or raw firmware image into tar. Gzip detection uses bytes rather than the extension. Supply two local regular files to the CLI, and put flags before filenames.

Unsupported entry kinds, global PAX headers, GNU/SCHILY sparse data and non-regular payloads fail. Malformed headers, truncated payloads, gzip checksum errors and nonzero trailing decoded bytes also fail. Recreate or recopy the archive from a trustworthy build output rather than interpreting a failed scan as a partial success. Do not strip metadata to hide an unsupported feature without understanding the resulting loss.

## Invalid paths and hardlinks

Absolute entry paths, traversal segments, invalid UTF-8 and duplicate normalized paths are invalid. `etc/device.conf` and `./etc//device.conf` name the same normalized entry and cannot both occur. An explicitly recorded non-directory cannot be another entry's ancestor. Fix archive creation rather than extracting the data to work around these checks.

Ignore and required-path values must already be canonical: no leading slash, `./`, repeated separators or trailing slash. A hardlink must end at a recorded regular entry; missing targets, cycles and directory/symlink endpoints fail. Symlink targets are preserved without following them, so an absolute symlink target is not by itself an invalid entry path.

## Limits

A limit failure identifies the exhausted budget. Go callers can inspect `*rootfsdiff.LimitError`; `Actual` is the first observed excess and may be smaller than the unread input's eventual total. Inspect the archive producer and choose a justified positive override if it is expected to be larger. Never treat a failed manifest as complete.

`--max-archive-bytes` counts decoded headers, extensions and padding as well as payloads, not the compressed file size. `--max-path-bytes` counts UTF-8 bytes independently for each decoded entry path/target, not characters. Go option zero means default; explicit CLI zero is invalid. See [limits](semantics.md#resource-limits).

## Metadata warnings and unexpected changes

`uncompared_metadata` names an encountered key whose value is not compared. It is a warning rather than a policy violation. Its absence does not change the standard exclusions listed in every report. Warning path `.` refers to metadata on an ignored root directory record. Warnings remain for ignored paths.

Check numeric UID/GID and the full `07777` mode bits when content appears unchanged. Hardlinks inherit terminal regular metadata/content, so one payload change can affect several paths. Adding/removing an explicit directory record is a change even if its children are unchanged. Timestamp or compression changes alone are excluded. Payload growth is not disk, flash, compressed archive or OTA transfer growth.

## Required paths and gate failures

A required path must be an explicit canonical candidate entry. An implicit parent directory does not count. A symlink counts as present without proving its target exists or can run. Ignores cannot bypass this check. Confirm the exact path spelling and candidate archive records.

Ordinary differences return 0 unless `--fail-on-change` or a policy fails. Exit 1 is a completed comparison with a failed enabled check and a complete report. A growth limit tests signed net growth after ignores; equality and shrinkage pass. Inspect `violations` for policy details and `changes` for fail-on-change results.

## Output and writer errors

Parse, scan and compare failures produce no report bytes, but shell redirection may have created an empty destination. Writer failures can leave partial JSON, Markdown or text and return exit 2. Examples include a full destination filesystem or a downstream reader that closes early. Check the command's exit code before parsing or promoting a report; do not equate file existence with completion.

Go callers must check `report.Write`'s error and any relevant file-close error. Underlying I/O errors remain discoverable through wrapping. For CI, avoid a final successful command or `tee` hiding the tool's exit; use the [status-preserving example](ci.md).
