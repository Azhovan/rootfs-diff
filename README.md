# rootfs-diff

A Go library and CLI for reviewing recorded root filesystem changes before a firmware release.

rootfs-diff compares two tar or tar.gz archives, reports content and metadata changes, and checks explicit project policies. It reads archives without extracting or executing their contents.

The scanner, comparison, policy evaluator, report renderers and CLI are implemented.

Use rootfs-diff to review changes between a baseline and candidate rootfs: removed paths, content changes, numeric ownership, permissions, link targets and device metadata. Regular-file payload growth is measured in uncompressed bytes. Ordinary differences succeed; an explicit policy or `--fail-on-change` enables a CI gate.

## Install and quickstart

Use Linux or macOS with Go **1.27.4** (module minimum **1.27.0**). The host binary's CPU architecture must match the machine running it; firmware architecture does not matter because archive contents are never executed. This repository is public; cloning and consuming the upstream module do not require GitHub credentials.

```sh
git clone https://github.com/Azhovan/rootfs-diff.git
cd rootfs-diff
./scripts/verify.sh
```

Build the command, then compare your baseline and candidate rootfs archives:

```sh
go build -o ./bin/rootfs-diff ./cmd/rootfs-diff
./bin/rootfs-diff baseline.tar candidate.tar.gz
./bin/rootfs-diff --format json baseline.tar candidate.tar.gz > report.json
./bin/rootfs-diff --policy policy.json baseline.tar candidate.tar.gz
```

Use the [sample policy](docs/cli.md#policy-format) for the last command, adapting it to your candidate. For a self-contained library example requiring no archive files:

```sh
go run ./examples/library
```

Put flags before both filenames. Exit **0** means the comparison and enabled checks passed, **1** means a configured gate failed, and **2** means invalid input or an operational failure. Exit 1 still produces a complete report. Treat output from exit 2 as incomplete.

## Documentation

- [Installation, module access and private copies](docs/installation.md)
- [CLI flags, policies, ignores and exits](docs/cli.md)
- [Archive metadata, paths, links, growth and limits](docs/semantics.md)
- [Go library API and ownership](docs/library.md), [example structure](examples/library/README.md)
- [Report formats and compatibility](docs/reports.md), [model fields](docs/models.md), [JSON schema](docs/report-v1.schema.json)
- [Ordinary GitHub Actions usage](docs/ci.md), [troubleshooting](docs/troubleshooting.md)
- [Contributor setup and local checks](CONTRIBUTING.md)

The [synthetic example](examples/synthetic/README.md) provides a small reproducible comparison. The [pinned Buildroot demonstration](examples/buildroot/README.md) records an executed two-image archive comparison, its controlled changes, prerequisites and observed report. These demonstrations review recorded archive data; they do not establish bootability, runtime correctness or OTA compatibility.

## Scope

v0.1 accepts local tar and tar.gz files, reads without extraction, and reports explicit recorded data. It excludes timestamps, user/group names, xattrs, ACLs, capabilities and SELinux labels. Reports declare these exclusions and warn about encountered metadata that is not compared. Sparse archives and unsupported input formats fail. See [semantics and limits](docs/semantics.md).

This tool does not establish bootability, runtime correctness, OTA compatibility, CVE applicability, compliance or complete security equivalence. It does not unpack raw firmware, mount filesystems, attribute packages or run target binaries.

[Yocto buildhistory](https://docs.yoctoproject.org/dev/dev-manual/build-quality.html) records build-system package and image information. [diffoscope](https://diffoscope.org/) provides broad, detailed artifact comparisons. rootfs-diff focuses on a small importable Go API, rootfs tar metadata and explicit project gates. It complements those tools.

The import path is `github.com/Azhovan/rootfs-diff`, package `rootfsdiff`. Development follows the [v0.1 contract](docs/v0.1-contract.md) and [delivery issue](https://github.com/Azhovan/rootfs-diff/issues/1).
