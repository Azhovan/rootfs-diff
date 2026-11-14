#!/bin/sh
set -eu

target=$1
case "${ROOTFSDIFF_VARIANT:-}" in
    before|after) ;;
    *) printf 'ROOTFSDIFF_VARIANT must be before or after\n' >&2; exit 1 ;;
esac

# Reset only this demonstration's paths. Reusing output/target must not leave
# the removed startup script behind when generating the candidate image.
rm -rf "$target/etc/rootfs-diff-demo" "$target/usr/share/rootfs-diff-demo"
rm -f "$target/etc/init.d/S99rootfs-diff-demo"
mkdir -p "$target/etc/rootfs-diff-demo" "$target/usr/share/rootfs-diff-demo"
printf 'demo executable\n' > "$target/usr/share/rootfs-diff-demo/tool"
chmod 0755 "$target/usr/share/rootfs-diff-demo/tool"
if [ "$ROOTFSDIFF_VARIANT" = before ]; then
    printf '#!/bin/sh\nexit 0\n' > "$target/etc/init.d/S99rootfs-diff-demo"
    chmod 0755 "$target/etc/init.d/S99rootfs-diff-demo"
    printf 'feature=off\n' > "$target/etc/rootfs-diff-demo/config"
    printf 'base\n' > "$target/usr/share/rootfs-diff-demo/data"
    ln -s config "$target/etc/rootfs-diff-demo/current"
else
    printf 'feature=on!\n' > "$target/etc/rootfs-diff-demo/config"
    printf 'base\nadditional payload\n' > "$target/usr/share/rootfs-diff-demo/data"
    chmod 0700 "$target/usr/share/rootfs-diff-demo/tool"
    ln -s /etc/rootfs-diff-demo/config "$target/etc/rootfs-diff-demo/current"
fi
chmod 0644 "$target/etc/rootfs-diff-demo/config" "$target/usr/share/rootfs-diff-demo/data"
