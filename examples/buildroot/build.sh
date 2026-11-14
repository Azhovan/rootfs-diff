#!/bin/sh
set -eu

# Run only in the pinned Linux builder, as its ordinary br-user account.
work=/tmp/rootfs-diff-buildroot
jobs=${BUILDROOT_JOBS:-2}
cd "$work"
mkdir -p artifacts dl
tar -xJf buildroot-2025.02.19.tar.xz
cd buildroot-2025.02.19
make O="$work/output" BR2_DEFCONFIG="$work/recipe/defconfig" defconfig
make O="$work/output" BR2_DL_DIR="$work/dl" source
printf '\nBuilding baseline rootfs\n'
ROOTFSDIFF_VARIANT=before make O="$work/output" BR2_DL_DIR="$work/dl" -j"$jobs"
cp "$work/output/images/rootfs.tar" "$work/artifacts/before.tar"
printf '\nBuilding candidate rootfs with the same configuration/toolchain\n'
ROOTFSDIFF_VARIANT=after make O="$work/output" BR2_DL_DIR="$work/dl" -j"$jobs"
cp "$work/output/images/rootfs.tar" "$work/artifacts/after.tar"
cp "$work/output/.config" "$work/artifacts/buildroot.config"
