# rootfs-diff

A Go library and CLI in development for reviewing recorded root filesystem changes before a firmware release.

The v0.1 target compares two tar or tar.gz archives, reports content and metadata changes, and checks explicit project policies. It reads archives without extracting or executing their contents.

**Status:** the shared models, validation, schema examples and archive test fixtures are implemented. Scanning, comparison, policy evaluation, rendering and the CLI are assigned to subsequent implementation issues and are not yet available.

- [Delivery plan and linked implementation issues](https://github.com/Azhovan/rootfs-diff/issues/1)
- [v0.1 behavior and interface contract](docs/v0.1-contract.md)
- [Model and JSON schema reference](docs/models.md)
- [Contributor setup and commands](CONTRIBUTING.md)

The module is `github.com/Azhovan/rootfs-diff`, package `rootfsdiff`. The repository remains private. This compares recorded archive data; it does not establish bootability, runtime correctness, security or update compatibility.
