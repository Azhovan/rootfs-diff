# Executed Buildroot artifact comparison

This separate, slow example builds two rootfs tar archives with Buildroot and
compares them using the actual rootfs-diff executable. It does not boot an image,
execute target files, mount a filesystem, or establish runtime correctness,
security, hardware compatibility, or general Buildroot/Yocto compatibility.
The small [synthetic example](../synthetic/README.md) is the ordinary offline demo.
Neither `scripts/verify.sh` nor the integration tests run this Buildroot recipe.

## Prerequisites and reproduction

Use an installed Go 1.27.4 toolchain active on `PATH`, Git, Python 3.11 or later,
Bash, curl, tar, and Docker with a running Linux arm64 builder. This recipe pins an arm64 container;
the observed run used native arm64 Linux in a local Colima VM on macOS arm64.
An x86 host would need working arm64 emulation, which this example has not tested.
The builder runs as UID/GID 1000, without privileged mode or host mounts. Allow
4 GiB builder RAM, local CPU time, and at least 20 GiB free Docker storage for the
toolchain/build tree. Downloads and the builder image require network access on
the first run. There are no paid/cloud resources or hardware prerequisites.

From the repository root, choose a new absolute output directory outside Git:

```sh
DOCKER_CONTEXT=colima-wifx-case2 \
  ./examples/buildroot/run.sh /tmp/rootfs-diff-buildroot-proof
```

Omit `DOCKER_CONTEXT` when the current Docker context is already suitable. The
script refuses to overwrite proof artifacts. `BUILDROOT_JOBS` defaults to 2;
`BUILDROOT_CPUS` defaults to 3 and `BUILDROOT_MEMORY` to `4g`. Existing containers
are not changed. Follow the container name and build-log path printed at startup.

The exact observed command reused a preverified source cache and disabled the
build container's network:

```sh
BUILDROOT_SOURCE=/private/tmp/rootfs-diff-buildroot-prereq.2PRTHm/buildroot-2025.02.19.tar.xz \
BUILDROOT_DL_CACHE=/private/tmp/rootfs-diff-buildroot-prereq.2PRTHm/dl \
BUILDROOT_NETWORK=none DOCKER_CONTEXT=colima-wifx-case2 BUILDROOT_JOBS=3 \
  ./examples/buildroot/run.sh /private/tmp/rootfs-diff-buildroot-demo-20261115
```

Those temporary paths are execution evidence, not required installation paths.
For offline reuse, `BUILDROOT_SOURCE` selects the release archive and
`BUILDROOT_DL_CACHE` selects a Buildroot download-cache directory with the layout
shown below. The image must already be cached. `docker pull` still checks the
pinned image reference; archive compilation itself needs no network when the
complete cache is provided. All source hashes are checked by Buildroot before use.

`run.sh` builds the CLI from the current checkout, embeds its commit as the
version, builds both archives through `build.sh`, saves JSON/text reports and
checks exits 0/1, same-image comparison, repeated report bytes, and independent
tar inspection. All flags precede archive paths. The comparison commands are:

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
complete report, including the readable missing-file and growth-budget results.
`run.sh` asserts its JSON gate outcomes rather than treating a failed command as
an empty successful report. The synthetic demo separately exercises exit 2.

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

The checked-in [defconfig](defconfig) selects aarch64, an internal musl toolchain,
BusyBox, reproducible builds, and uncompressed tar. It builds no kernel image,
bootloader, or QEMU. Linux sources are still needed for toolchain headers.
Buildroot expands defaults from this fixed release, and the resolved
`buildroot.config` is retained beside the generated artifacts.

Both builds use the same configuration and fixed container work path
`/tmp/rootfs-diff-buildroot`. [post-build.sh](post-build.sh) receives the variant
through `ROOTFSDIFF_VARIANT`; it explicitly owns the four demonstration paths
and two unchanged endpoint scripts, and removes `S40network` before creating
the candidate. Its baseline startup script is a controlled stub, not the
Buildroot networking implementation. One build tree avoids
unrelated compiler differences while still generating both archives through
Buildroot's fakeroot/tar image step. No rootfs archive is unpacked onto the host.

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

This real Buildroot filesystem contains deliberately controlled demonstration
payloads at familiar release-review paths. `libexample.so` consists of ordinary,
fully written zero bytes, not an ELF/shared-library binary. The `agent-v1` and
`agent-v2` endpoints are unchanged shell-script stubs, not a production device
agent. Neither target programs nor scripts are executed. The baseline
`S40network` is explicitly replaced with this example's 17-byte startup stub,
so the removal finding does not depend on incidental Buildroot defaults.

All fixture paths have numeric UID/GID 0. Modes below are octal. These are the
four independently specified changes, not a hand-written imitation of CLI text:

| Path | Before | After | Expected fields |
| --- | --- | --- | --- |
| `etc/init.d/S40network` | mode 0755, `#!/bin/sh\nexit 0\n` (17 bytes) | absent | removed |
| `etc/device.conf` | mode 0640, `device=demo\n` (12 bytes) | same bytes, mode 0666 | mode |
| `usr/bin/device-agent` | symlink `agent-v1` | symlink `agent-v2` | link_target |
| `usr/lib/libexample.so` | mode 0644, 1024 zero bytes | mode 0644, 431104 zero bytes | size, sha256 |

The link endpoints `usr/bin/agent-v1` and `usr/bin/agent-v2` are explicit regular
entries, both mode 0755, present and unchanged in both images. Their exact
contents are `#!/bin/sh\n# demo agent-v1\nexit 0\n` and the corresponding `v2`
string. Symlink usability is not inferred by the production scanner.

Expected: one removal, three modifications, no additions. Library-named payload
growth is exactly `420 * 1024 = 430080` bytes; net regular-payload growth is
`430080 - 17 = 430063` bytes because the startup script was removed. The one
positive-growth entry is `usr/lib/libexample.so` at 430080 bytes. These values
are neither filesystem/flash usage nor an ELF/ABI measurement.

The checked-in policy is user-defined CI configuration: it requires
`etc/init.d/S40network` and permits at most 409600 bytes (400 KiB) of net payload
growth. This candidate therefore fails both `required_path` and
`max_total_growth_bytes`. Presence and growth are the only enabled rules; the
permission and symlink findings are reported for human review without an
inferred security policy. To gate only on the missing file, omit
`max_total_growth_bytes` from a copy of the policy. Ordinary differences exit 0
unless a gate is enabled.

No other entry may change. The independent Python inspector reads every tar
record and hashes regular payloads without extraction or the production scanner;
it checks all fixture metadata, unchanged endpoints, the entire JSON report,
and any additional differences. It does not validate boot or runtime behavior.

## Observed execution

The documented fresh-build command completed end to end on 2026-11-15 using
rootfs-diff compiled from commit `0f39dc8d42fe8aa6c8ff59b52ea33eee06c08f19`,
the immutable v0.1.0 release source. Only non-Go example files were modified at
CLI compilation. The new non-root arm64 container used the pinned image,
17 verified cached sources, three CPUs, a 4 GiB memory cap, no host mounts, and
no build network. It retained about 6.4 GiB of inputs/build state. This was a
fresh toolchain/build tree, followed by baseline and candidate fakeroot/tar
image generations through Buildroot; no existing rootfs tar was edited.

| Artifact | Bytes | SHA-256 |
| --- | ---: | --- |
| `before.tar` | 2283520 | `fb40c3185b3fd1d1776dbdd07f78224a4a80825c71edb831a8bdd1bece3c855c` |
| `after.tar` | 2713600 | `1cb97e7c856f23eaa151bcec339835f382b6243aca92e5b2e947d76b848fbf1a` |
| `buildroot.config` | 128632 | `18239d37ffed0fafcb40f8a0f25aba23df076e9c2ddcf4301c2542e8789510be` |

The ordinary comparison exited 0. Baseline versus itself with
`--fail-on-change` exited 0 with 369 unchanged entries, no changes, and zero
growth. Both the policy comparison and candidate `--fail-on-change` exited 1.
Repeated ordinary JSON and policy-enabled text bytes were identical. The
independent checker verified every tar entry, full normal/same/gated JSON
report, payload hash and total, endpoint fixture, and archive hash. The result
was exactly the four expected changes, no additions, no additional differences,
and no metadata warnings. No ignore rules suppressed anything.

The policy-enabled text command shown above also ran and exited 1 with empty
stderr. This is its complete, literal CLI output, including both user-defined
CI failures:

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

Archive hashes identify this execution. Pinned inputs and fixed build paths
improve reproducibility, but this execution does not establish bit-identical
outputs across other hosts or Buildroot releases. The finding about
`libexample.so` measures recorded payload growth in deliberately non-ELF data;
it says nothing about ABI compatibility, runtime behavior, or bootability.
The previous demonstration's source and documented results remain available in the
immutable v0.1.0 tag rather than being relabeled as this new execution.

## Cleanup

Keep the output directory and printed container until review is complete. The
directory contains archives, reports, inspection results, the resolved config,
CLI, release source, and build log. The retained container contains the large
build tree and download cache. Remove only the named proof container using the
printed `docker rm -f` command, then remove the chosen output directory. No
generated firmware, toolchain, third-party binary, or build tree belongs in Git.
