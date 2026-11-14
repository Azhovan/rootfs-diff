# rootfs-diff

A planned Go library and CLI for reviewing embedded Linux root filesystem changes before a firmware release.

The v0.1 target compares two tar or tar.gz archives, reports content and metadata changes, and checks explicit project policies. It reads archives without extracting or executing their contents.

**Status:** planning only. The application and its tests have not been implemented yet.

- [Delivery plan and linked implementation issues](https://github.com/Azhovan/rootfs-diff/issues/1)
- [v0.1 behavior and interface contract](docs/v0.1-contract.md)
- [Instructions for implementation agents](AGENTS.md)

Implementation will happen directly on `main`. Start with issue #1, then take an unblocked sub-issue. The repository remains private.
