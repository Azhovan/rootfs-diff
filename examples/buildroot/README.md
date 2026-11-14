# Executed Buildroot artifact comparison

This separate, slow example builds two rootfs tar archives with Buildroot and
compares them using the actual rootfs-diff executable. It does not boot an image,
execute target files, mount a filesystem, or establish runtime correctness,
security, hardware compatibility, or general Buildroot/Yocto compatibility.
The small [synthetic example](../synthetic/README.md) is the ordinary offline demo.
Neither `scripts/verify.sh` nor the integration tests run this Buildroot recipe.

## Prerequisites and reproduction

Use an installed Go 1.27.4 toolchain, Git, Python 3.11 or later, Bash, curl, tar,
and Docker with a running Linux arm64 builder. This recipe pins an arm64 container;
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
  ./examples/buildroot/run.sh /private/tmp/rootfs-diff-buildroot-proof-20261114
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
python3 examples/buildroot/check.py "$OUTPUT/before.tar" "$OUTPUT/after.tar" \
  "$OUTPUT/report.json" "$OUTPUT/same.json" "$OUTPUT/gated.json"
```

The first two commands return 0; the policy command returns 1 with a complete
report. `run.sh` asserts those outcomes rather than treating a failed command as
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
through `ROOTFSDIFF_VARIANT`; it resets only project-owned paths and explicitly
removes the startup script before creating the candidate. One build tree avoids
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

All demo paths have numeric UID/GID 0. The regular-file contents below are exact
UTF-8 bytes with the shown newlines; symlink targets are compared verbatim.

| Path | Before | After | Expected changed fields |
| --- | --- | --- | --- |
| `etc/init.d/S99rootfs-diff-demo` | mode 0755, `#!/bin/sh\nexit 0\n` (17 bytes) | absent | removed |
| `etc/rootfs-diff-demo/config` | mode 0644, `feature=off\n` (12 bytes) | `feature=on!\n` (12 bytes) | sha256 |
| `etc/rootfs-diff-demo/current` | symlink `config` | symlink `/etc/rootfs-diff-demo/config` | link_target |
| `usr/share/rootfs-diff-demo/data` | mode 0644, `base\n` (5 bytes) | `base\nadditional payload\n` (24 bytes) | size, sha256 |
| `usr/share/rootfs-diff-demo/tool` | mode 0755, `demo executable\n` (16 bytes) | same bytes, mode 0700 | mode |

Expected: one removal, four modifications, no additions, net payload growth
`+19 - 17 = +2` bytes, and one positive-growth entry of 19 bytes for `data`.
The policy requires the removed startup script and permits only 1 byte net
growth, so both `required_path` and `max_total_growth_bytes` must fail. No other
entry may change. The inspector reads every tar record and hashes payloads using
Python's standard library, without extraction or the production scanner; it
checks the entire report and flags any additional difference.

## Observed execution

The command above completed end to end on 2026-11-14 with rootfs-diff built from
accepted CLI commit `9f82ac32fd764392ebe7ad479197c91b4b4a55ea`. The only local
changes at CLI compilation were this example's non-Go recipe files. The builder
had networking disabled and used the 17 verified cached sources. Both rootfs
archives were produced by Buildroot, including the candidate's second fakeroot
image step. The isolated container retained about 6.4 GiB of inputs/build state;
the run took about eight minutes on three allocated arm64 CPUs. Actual time and
peak storage depend on host resources and configuration.

| Artifact | Bytes | SHA-256 |
| --- | ---: | --- |
| `before.tar` | 2293760 | `9fa91e802f3bd60cc5c6b4b7746f141ed9d336d2690e893795a88386267621fb` |
| `after.tar` | 2293760 | `82cfcba2243bebedf513c9a10e47d7e4d9a0712d892499a8f3c0b76bb514aa3b` |
| `buildroot.config` | 128632 | `18239d37ffed0fafcb40f8a0f25aba23df076e9c2ddcf4301c2542e8789510be` |

The normal comparison exited 0 and repeated JSON bytes matched. Baseline versus
itself with `--fail-on-change` exited 0 with 371 unchanged entries, zero changes,
and zero growth. Candidate `--fail-on-change` exited 1. The policy comparison
exited 1 with both expected violations: growth actual 2 versus limit 1, then
missing `etc/init.d/S99rootfs-diff-demo`. Python inspection independently checked
every recorded entry, regular payload hash/total, complete normal/same/gated
JSON report, and both artifact hashes. No extra compared differences or metadata
warnings occurred; none were hidden by ignore rules.

The complete observed ordinary text report was:

```text
rootfs-diff report (schema 1)

Summary:
  Entries: before 371, after 370
  Changes: added 0, removed 1, modified 4, unchanged 366
  Regular payload: before 1701303 bytes, after 1701305 bytes, net +2 bytes

Payload totals count retained regular entries only, excluding hardlink aliases; they are not filesystem or flash usage.

Positive growth (showing 1 of 1):
  "usr/share/rootfs-diff-demo/data": +19 bytes

Changes (5):
  removed "etc/init.d/S99rootfs-diff-demo"
    Before: kind=regular mode=0755 uid=0 gid=0 size=17 sha256="306c6ca7407560340797866e077e053627ad409277d1b9da58106fce4cf717cb"
    After: absent
  modified "etc/rootfs-diff-demo/config"
    Changed fields: sha256
    Before: kind=regular mode=0644 uid=0 gid=0 size=12 sha256="e372999e237ce6db7cfe16185902221c74c4f875eb2f239345d4a7712336fd6f"
    After: kind=regular mode=0644 uid=0 gid=0 size=12 sha256="62f9ab0dcedf785ebb2a379188877fd9fdfd66084c68dddac5ba0ef2611df7d5"
  modified "etc/rootfs-diff-demo/current"
    Changed fields: link_target
    Before: kind=symlink mode=0777 uid=0 gid=0 link_target="config"
    After: kind=symlink mode=0777 uid=0 gid=0 link_target="/etc/rootfs-diff-demo/config"
  modified "usr/share/rootfs-diff-demo/data"
    Changed fields: size, sha256
    Before: kind=regular mode=0644 uid=0 gid=0 size=5 sha256="f34848ca92665c342abd5816c9e3eda0e82180671195362bcd0080544a3bc2ac"
    After: kind=regular mode=0644 uid=0 gid=0 size=24 sha256="0890808af31800ff7d09d149176b25b167a721dc27ff8ece66744f84c14e71ae"
  modified "usr/share/rootfs-diff-demo/tool"
    Changed fields: mode
    Before: kind=regular mode=0755 uid=0 gid=0 size=16 sha256="133bcd54c5814c553cf545a8b0acc226e2356fccaa90af2937b33321d1f6f00f"
    After: kind=regular mode=0700 uid=0 gid=0 size=16 sha256="133bcd54c5814c553cf545a8b0acc226e2356fccaa90af2937b33321d1f6f00f"

Policy violations (0):
  None recorded.

Warnings (0):
  None recorded.

Metadata exclusions:
  acls, directory_sizes, file_capabilities, selinux_labels, timestamps, unrecognized_pax_metadata, user_group_names, xattrs

These metadata are not compared. No changes does not establish firmware equivalence, bootability, compatibility, or security.
```

Archive hashes identify this execution. The recipe pins inputs and fixes build
paths to improve reproducibility, but this single execution does not establish
bit-identical outputs across other build hosts, output paths, toolchains, or
Buildroot releases. The asserted result is the five controlled artifact changes
and independently checked totals, not a boot or security verdict.

## Cleanup

Keep the output directory and printed container until review is complete. The
directory contains archives, reports, inspection results, the resolved config,
CLI, release source, and build log. The retained container contains the large
build tree and download cache. Remove only the named proof container using the
printed `docker rm -f` command, then remove the chosen output directory. No
generated firmware, toolchain, third-party binary, or build tree belongs in Git.
