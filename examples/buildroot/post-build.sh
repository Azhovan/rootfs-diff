#!/bin/sh
set -eu

target=$1
case "${ROOTFSDIFF_VARIANT:-}" in
    before|after) ;;
    *) printf 'ROOTFSDIFF_VARIANT must be before or after\n' >&2; exit 1 ;;
esac

# Own these fixture paths explicitly, including the familiar startup filename.
# Neither Buildroot's default S40network contents nor stale output/target state
# determine the expected result. The endpoint scripts are unchanged demo stubs.
mkdir -p "$target/etc/init.d" "$target/usr/bin" "$target/usr/lib"
rm -f "$target/etc/init.d/S40network" "$target/usr/bin/device-agent"
printf 'device=demo\n' > "$target/etc/device.conf"
printf '#!/bin/sh\n# demo agent-v1\nexit 0\n' > "$target/usr/bin/agent-v1"
printf '#!/bin/sh\n# demo agent-v2\nexit 0\n' > "$target/usr/bin/agent-v2"
chmod 0755 "$target/usr/bin/agent-v1" "$target/usr/bin/agent-v2"
if [ "$ROOTFSDIFF_VARIANT" = before ]; then
    printf '#!/bin/sh\nexit 0\n' > "$target/etc/init.d/S40network"
    chmod 0755 "$target/etc/init.d/S40network"
    chmod 0640 "$target/etc/device.conf"
    ln -s agent-v1 "$target/usr/bin/device-agent"
    blocks=1
else
    chmod 0666 "$target/etc/device.conf"
    ln -s agent-v2 "$target/usr/bin/device-agent"
    blocks=421
fi
# Deliberately ordinary zero-filled bytes, not an ELF/shared-library binary.
# Write all blocks rather than creating a sparse file. Delta: 420 * 1024 bytes.
dd if=/dev/zero of="$target/usr/lib/libexample.so" bs=1024 count="$blocks" status=none
chmod 0644 "$target/usr/lib/libexample.so"
