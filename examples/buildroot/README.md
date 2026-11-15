# Executed Buildroot artifact comparison

This example builds two root filesystem tar archives with Buildroot and compares
them using the rootfs-diff executable. It is a separate, slower demonstration;
neither `scripts/verify.sh` nor the integration tests run it. For a small offline
demo, use the [synthetic example](../synthetic/README.md).

The example compares recorded archive data. It does not boot an image, execute
target files or mount a filesystem. Its results do not establish runtime
correctness, security, hardware compatibility or general compatibility with
Buildroot and Yocto outputs.

## Prerequisites and reproduction

Install Go 1.27.4 and make sure it is active on `PATH`. You also need Git,
Python 3.11 or later, Bash, curl, tar, and Docker with a running Linux arm64
builder. The recipe pins an arm64 container image. The recorded run used native
arm64 Linux in a local Colima VM on macOS arm64. An x86 host would need arm64
emulation, which has not been tested for this example.

The builder runs as UID/GID 1000, without privileged mode or host mounts. Allow
4 GiB of builder RAM, time for compilation, and at least 20 GiB of free Docker
storage for the toolchain and build tree. The first run needs network access to
download sources and the builder image. No paid services, cloud resources or
target hardware are required.

From the repository root, choose a new absolute output directory outside Git:

```sh
DOCKER_CONTEXT=colima-wifx-case2 \
  ./examples/buildroot/run.sh /tmp/rootfs-diff-buildroot-proof
```

Omit `DOCKER_CONTEXT` when the current Docker context is already suitable. The
script refuses to overwrite existing demonstration outputs. `BUILDROOT_JOBS`
defaults to 2; `BUILDROOT_CPUS` defaults to 3 and `BUILDROOT_MEMORY` to `4g`.
Existing containers
are not changed. The script prints the container name and build-log path at startup.

The recorded run used the following command. It reused a verified source cache
and disabled networking in the build container:

```sh
BUILDROOT_SOURCE=/private/tmp/rootfs-diff-buildroot-prereq.2PRTHm/buildroot-2025.02.19.tar.xz \
BUILDROOT_DL_CACHE=/private/tmp/rootfs-diff-buildroot-prereq.2PRTHm/dl \
BUILDROOT_NETWORK=none DOCKER_CONTEXT=colima-wifx-case2 BUILDROOT_JOBS=3 \
  ./examples/buildroot/run.sh /private/tmp/rootfs-diff-buildroot-demo-20261115
```

Those temporary paths identify the recorded run. Choose your own paths when
reproducing it. For offline reuse, `BUILDROOT_SOURCE` selects the release archive and
`BUILDROOT_DL_CACHE` selects a Buildroot download-cache directory with the layout
shown below. The container image must already be cached. `docker pull` still
checks the pinned image reference; compilation itself needs no network when
the complete source cache is supplied. Buildroot checks every source hash before use.

`run.sh` builds the CLI from the current checkout and embeds the commit in its
version string. It then calls `build.sh` to build both archives and saves JSON
and text reports. It checks exits 0 and 1, compares the baseline with itself,
verifies identical report bytes across repeated runs, and independently
inspects the tar archives. All flags precede archive paths. The comparison
commands are:

```sh
OUTPUT=/tmp/rootfs-diff-buildroot-proof
"$OUTPUT/rootfs-diff" --format json "$OUTPUT/before.tar" "$OUTPUT/after.tar"
"$OUTPUT/rootfs-diff" --format json --fail-on-change "$OUTPUT/before.tar" "$OUTPUT/before.tar"
"$OUTPUT/rootfs-diff" --format json --policy examples/buildroot/policy.json \
  "$OUTPUT/before.tar" "$OUTPUT/after.tar"
"$OUTPUT/rootfs-diff" --format text --policy examples/buildroot/policy.json \
  "$OUTPUT/before.tar" "$OUTPUT/after.tar"
python3 examples/buildroot/check.py "$OUTPUT/before.tar" "$OUTPUT/after.tar" \
  "$OUTPUT/report.json" "$OUTPUT/same.json" "$OUTPUT/gated.json"
```

The first two comparisons return 0; both policy comparisons return 1 with a
complete report showing the missing file and exceeded growth budget. `run.sh`
checks both the exit status and the JSON report for policy failures. The
synthetic demo separately tests exit 2.

## Pinned recipe and inputs

Release: [Buildroot 2025.02.19 LTS](https://buildroot.org/downloads/buildroot-2025.02.19.tar.xz).
SHA-256: `e5e58bff1da91f022f60cefdfe280140c08023cd0aeec1e5f6f9da0ea866d69c`.
The [official signed checksum statement](https://buildroot.org/downloads/buildroot-2025.02.19.tar.xz.sign)
was verified with signer fingerprint
`18C7DF2819C1733D822D599EA500D6EE9CB0E540`, using the public key linked from that
statement. GNU package hashes below also matched the release's SHA-512 entries
where provided; all listed SHA-256 values were independently computed.

Container: `registry.gitlab.com/buildroot.org/buildroot/base@sha256:429e702ce23ac7299b279d73a7dd7aa6eac2b9ebf126ffa482c131671ee2589c`.
This is the native arm64 manifest for upstream's `20250218.2110` builder tag,
with Debian GCC 12.2, GNU Make 4.3, and GNU tar 1.35. Pulling by digest fixes
container identity instead of relying on a mutable tag.

The checked-in [Buildroot configuration](defconfig) selects aarch64, an internal
musl toolchain, BusyBox, reproducible builds and uncompressed tar. It builds no kernel image,
bootloader, or QEMU. Linux sources are still needed for toolchain headers.
Buildroot fills in defaults from the pinned release, and the complete
`buildroot.config` is retained beside the generated artifacts.

Both builds use the same configuration and fixed container work path
`/tmp/rootfs-diff-buildroot`. [post-build.sh](post-build.sh) receives the variant
through `ROOTFSDIFF_VARIANT`. The script controls the four demonstration paths
and two unchanged scripts that serve as symlink targets. It ensures that
`S40network` is absent from the candidate. The baseline startup script is a controlled stub
created for this example. It is not Buildroot's networking implementation.
Reusing one build tree avoids unrelated compiler differences while generating
both archives through Buildroot's fakeroot/tar image step. No rootfs archive is
unpacked onto the host.

The complete source cache for this configuration and pinned builder contains:

| Cache path | SHA-256 |
| --- | --- |
| `acl/acl-2.3.2.tar.xz` | `97203a72cae99ab89a067fe2210c1cbf052bc492b479eca7d226d9830883b0bd` |
| `attr/attr-2.5.2.tar.xz` | `f2e97b0ab7ce293681ab701915766190d607a1dba7fae8a718138150b700a70b` |
| `autoconf/autoconf-2.72.tar.xz` | `ba885c1319578d6c94d46e9b0dceb4014caafe2490e437a0dbca3f270a223f5a` |
| `automake/automake-1.16.5.tar.xz` | `f01d58cd6d9d77fbdca9eb4bbd5ead1988228fdb73d6f7a201f5f8d6b118b469` |
| `binutils/binutils-2.43.1.tar.xz` | `13f74202a3c4c51118b797a39ea4200d3f6cfbe224da6d1d95bb938480132dfd` |
| `busybox/busybox-1.37.0.tar.bz2` | `3311dff32e746499f4df0d5df04d7eb396382d7e108bb9250e7b519b837043a4` |
| `fakeroot/fakeroot_1.36.orig.tar.gz` | `7fe3cf3daf95ee93b47e568e85f4d341a1f9ae91766b4f9a9cdc29737dea4988` |
| `gcc/gcc-13.4.0.tar.xz` | `9c4ce6dbb040568fdc545588ac03c5cbc95a8dbf0c7aa490170843afb59ca8f5` |
| `gmp/gmp-6.3.0.tar.xz` | `a3c2b80201b89e68616f4ad30bc66aee4927c3ce50e33929ca819d5c43538898` |
| `libtool/libtool-2.4.6.tar.xz` | `7c87a8c2c8c0fc9cd5019e402bed4292462d00a718a7cd5f11218153bf28b26f` |
| `linux/linux-6.12.112.tar.xz` | `164dc9d1f6c93c61a15e1f071c48379b467f2b17c469cce7223471968208ed03` |
| `m4/m4-1.4.21.tar.xz` | `f25c6ab51548a73a75558742fb031e0625d6485fe5f9155949d6486a2408ab66` |
| `mpc/mpc-1.3.1.tar.gz` | `ab642492f5cf882b74aa0cb730cd410a81edcdbec895183ce930e706c1c759b8` |
| `mpfr/mpfr-4.1.1.tar.xz` | `ffd195bd567dbaffc3b98b23fd00aad0537680c9896171e44fe3ff79e28ac33d` |
| `musl/musl-1.2.6.tar.gz` | `d585fd3b613c66151fc3249e8ed44f77020cb5e6c1e635a616d3f9f82460512a` |
| `musl-compat-headers/queue.h` | `c13407edd0e33be73cae72514cb234f8612e1c0e54401c9448daffd3a240158b` |
| `patchelf/patchelf-0.13.tar.bz2` | `4c7ed4bcfc1a114d6286e4a0d3c1a90db147a4c3adda1814ee0eee0f9ee917ed` |

`queue.h` comes from NetBSD commit `03be82a6b173b3c62116b7a186067fed3004dd44`.
The remaining package helpers, patches, skeleton, and `cdefs.h` are bundled in
the pinned Buildroot source. The downloaded inputs total 280,825,726 bytes.

## Independently expected findings

The generated Buildroot filesystem contains controlled demonstration files at
paths typical of a release review. `libexample.so` consists of ordinary,
fully written zero bytes, not an ELF/shared-library binary. The `agent-v1` and
`agent-v2` endpoints are unchanged shell-script stubs, not a production device
agent. No target programs or scripts are executed. The baseline
`S40network` is explicitly replaced with this example's 17-byte startup stub,
so the removal finding does not depend on incidental Buildroot defaults.

All demonstration paths have numeric UID/GID 0. Modes below are octal. The four
expected changes were specified independently of the CLI output:

| Path | Before | After | Expected fields |
| --- | --- | --- | --- |
| `etc/init.d/S40network` | mode 0755, `#!/bin/sh\nexit 0\n` (17 bytes) | absent | removed |
| `etc/device.conf` | mode 0640, `device=demo\n` (12 bytes) | same bytes, mode 0666 | mode |
| `usr/bin/device-agent` | symlink `agent-v1` | symlink `agent-v2` | link_target |
| `usr/lib/libexample.so` | mode 0644, 1024 zero bytes | mode 0644, 431104 zero bytes | size, sha256 |

The symlink targets `usr/bin/agent-v1` and `usr/bin/agent-v2` are explicitly
recorded regular files. Both have mode 0755 and are unchanged between the
images. Their exact
contents are `#!/bin/sh\n# demo agent-v1\nexit 0\n` and the corresponding `v2`
string. The production scanner records the symlink target without checking
whether the link is usable.

The expected result is one removal, three modifications and no additions.
The file named `libexample.so` grows by exactly `420 * 1024 = 430080` bytes.
Net regular-file payload growth is
`430080 - 17 = 430063` bytes because the startup script was removed. The one
positive-growth entry is `usr/lib/libexample.so` at 430080 bytes. These values
measure uncompressed file contents. They do not measure filesystem or flash
usage, or reveal anything about ELF files or ABI compatibility.

The checked-in policy defines two checks for CI. It requires
`etc/init.d/S40network` and permits at most 409600 bytes (400 KiB) of net payload
growth. This candidate therefore fails both `required_path` and
`max_total_growth_bytes`. Presence and growth are the only enabled rules; the
permission and symlink changes are reported for human review; they do not
trigger a security rule. To fail only on the missing file, omit
`max_total_growth_bytes` from a copy of the policy. Ordinary differences exit 0
unless an enabled check requires failure.

No other entry may change. The independent Python inspector reads every tar
record and hashes regular-file contents without extracting entries or calling
the production scanner. It checks all demonstration metadata, unchanged symlink
targets, the entire JSON report, and any additional differences. It does not
validate boot or runtime behavior.

## Observed execution

The fresh-build command shown above completed on 2026-11-15 using
rootfs-diff compiled from commit `0f39dc8d42fe8aa6c8ff59b52ea33eee06c08f19`,
the immutable v0.1.0 release source. Only non-Go example files were modified at
the time the CLI was compiled. The new non-root arm64 container used the pinned image,
17 verified cached sources, three CPUs, a 4 GiB memory cap, no host mounts, and
no network during compilation. It retained about 6.4 GiB of source inputs and
build state. The run built a fresh toolchain and build tree, then generated
baseline and candidate images through Buildroot's fakeroot/tar step. It did not
edit an existing rootfs tar archive.

| Artifact | Bytes | SHA-256 |
| --- | ---: | --- |
| `before.tar` | 2283520 | `fb40c3185b3fd1d1776dbdd07f78224a4a80825c71edb831a8bdd1bece3c855c` |
| `after.tar` | 2713600 | `1cb97e7c856f23eaa151bcec339835f382b6243aca92e5b2e947d76b848fbf1a` |
| `buildroot.config` | 128632 | `18239d37ffed0fafcb40f8a0f25aba23df076e9c2ddcf4301c2542e8789510be` |

The ordinary comparison exited 0. Baseline versus itself with
`--fail-on-change` exited 0 with 369 unchanged entries, no changes, and zero
growth. Both the policy comparison and candidate `--fail-on-change` exited 1.
Repeated runs produced identical ordinary JSON reports and identical
policy-enabled text reports. The independent checker verified every tar entry,
the complete ordinary, same-image and policy-enabled JSON reports, payload hashes
and totals, the unchanged symlink targets, and archive hashes. The result
was exactly the four expected changes, no additions, no additional differences,
and no metadata warnings. No ignore rules suppressed anything.

The policy-enabled text command shown above also ran and exited 1 without writing
to stderr. Its complete CLI output follows, including both policy violations:

```text
rootfs-diff report (schema 1)

Summary:
  Entries: before 369, after 368
  Changes: added 0, removed 1, modified 3, unchanged 365
  Regular payload: before 1701934 bytes, after 2131997 bytes, net +430063 bytes

Payload totals count retained regular entries only, excluding hardlink aliases; they are not filesystem or flash usage.

Positive growth (showing 1 of 1):
  "usr/lib/libexample.so": +430080 bytes

Changes (4):
  modified "etc/device.conf"
    Changed fields: mode
    Before: kind=regular mode=0640 uid=0 gid=0 size=12 sha256="d370b5e9d3bb3df60181267a07d33b0258ae725346bf6689416d76c643129ab7"
    After: kind=regular mode=0666 uid=0 gid=0 size=12 sha256="d370b5e9d3bb3df60181267a07d33b0258ae725346bf6689416d76c643129ab7"
  removed "etc/init.d/S40network"
    Before: kind=regular mode=0755 uid=0 gid=0 size=17 sha256="306c6ca7407560340797866e077e053627ad409277d1b9da58106fce4cf717cb"
    After: absent
  modified "usr/bin/device-agent"
    Changed fields: link_target
    Before: kind=symlink mode=0777 uid=0 gid=0 link_target="agent-v1"
    After: kind=symlink mode=0777 uid=0 gid=0 link_target="agent-v2"
  modified "usr/lib/libexample.so"
    Changed fields: size, sha256
    Before: kind=regular mode=0644 uid=0 gid=0 size=1024 sha256="5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
    After: kind=regular mode=0644 uid=0 gid=0 size=431104 sha256="4573bfb5a93feb21d721fc7a5b9044aa065c53e9d97396368ac8c7c391078682"

Policy violations (2):
  max_total_growth_bytes: net growth 430063 bytes exceeds limit 409600 bytes
  required_path: missing explicit candidate path "etc/init.d/S40network"

Warnings (0):
  None recorded.

Metadata exclusions:
  acls, directory_sizes, file_capabilities, selinux_labels, timestamps, unrecognized_pax_metadata, user_group_names, xattrs

These metadata are not compared. No changes does not establish firmware equivalence, bootability, compatibility, or security.
```

The archive hashes identify this run. Pinned inputs and fixed build paths help
reproduce it, but do not establish byte-identical outputs across other hosts or
Buildroot releases. The finding about
`libexample.so` measures recorded payload growth in deliberately non-ELF data;
it says nothing about ABI compatibility, runtime behavior, or bootability.
The previous demonstration's source and documented results remain available in the
immutable v0.1.0 tag rather than being relabeled as this new execution.

## Cleanup

Keep the output directory and the container named in the script's output until
review is complete. The directory contains archives, reports, inspection
results, the complete configuration, CLI, release source and build log. The
retained container contains the large build tree and download cache. Remove only
that container using the printed `docker rm -f` command, then remove the chosen
output directory. No
generated firmware, toolchain, third-party binary, or build tree belongs in Git.
