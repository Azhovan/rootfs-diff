#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
if [ "$#" -ne 1 ]; then
    printf 'Usage: %s ABSOLUTE_OUTPUT_DIRECTORY\n' "$0" >&2
    exit 2
fi
case "$1" in /*) ;; *) printf 'Output directory must be absolute\n' >&2; exit 2 ;; esac
mkdir -p "$1"
out=$(CDPATH= cd -- "$1" && pwd)
shopt -s nullglob dotglob
existing=("$out"/*)
shopt -u nullglob dotglob
if [ "${#existing[@]}" -ne 0 ]; then
    printf 'Output directory must be empty: %s\n' "$out" >&2
    exit 2
fi
for tool in docker curl python3 go tar; do command -v "$tool" >/dev/null; done
image=registry.gitlab.com/buildroot.org/buildroot/base@sha256:429e702ce23ac7299b279d73a7dd7aa6eac2b9ebf126ffa482c131671ee2589c
source_sha=e5e58bff1da91f022f60cefdfe280140c08023cd0aeec1e5f6f9da0ea866d69c
source_file=$out/buildroot-2025.02.19.tar.xz
if [ -n "${BUILDROOT_SOURCE:-}" ]; then
    cp "$BUILDROOT_SOURCE" "$source_file"
else
    curl --fail --location --retry 2 --output "$source_file" \
        https://buildroot.org/downloads/buildroot-2025.02.19.tar.xz
fi
python3 - "$source_file" "$source_sha" <<'PY'
import hashlib, sys
with open(sys.argv[1], 'rb') as source:
    actual = hashlib.file_digest(source, 'sha256').hexdigest()
if actual != sys.argv[2]:
    raise SystemExit('Buildroot source SHA-256 mismatch: ' + actual)
print('Buildroot source SHA-256 verified: ' + actual)
PY
commit=$(git -C "$root" rev-parse HEAD)
# Resolve an already installed toolchain before disabling network lookup. The
# automatic toolchain wrapper cannot bootstrap itself with GOSUMDB disabled.
go_root=$(cd "$root" && go env GOROOT)
(cd "$root" && GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$go_root/bin/go" build -ldflags "-X main.version=$commit" -o "$out/rootfs-diff" ./cmd/rootfs-diff)
docker pull --platform linux/arm64 "$image"
container=rootfs-diff-buildroot-$$
printf 'Build container: %s\n' "$container"
printf 'Build log: %s/build.log\n' "$out"
# Retain this isolated container, including its build tree, for review/debugging.
# No privileged mode, host mounts, target execution, or existing container edits.
docker run --detach --name "$container" --platform linux/arm64 \
    --cpus "${BUILDROOT_CPUS:-3}" --memory "${BUILDROOT_MEMORY:-4g}" \
    --network "${BUILDROOT_NETWORK:-bridge}" "$image" sleep infinity >/dev/null
printf 'Cleanup after inspection: docker rm -f %s\n' "$container"
docker exec "$container" mkdir -p /tmp/rootfs-diff-buildroot/recipe /tmp/rootfs-diff-buildroot/dl
cat "$source_file" | docker exec -i "$container" sh -c 'cat > /tmp/rootfs-diff-buildroot/buildroot-2025.02.19.tar.xz'
tar --no-xattrs -C "$root/examples/buildroot" -cf - . | \
    docker exec -i "$container" tar --no-same-owner -xf - -C /tmp/rootfs-diff-buildroot/recipe
if [ -n "${BUILDROOT_DL_CACHE:-}" ]; then
    tar --no-xattrs -C "$BUILDROOT_DL_CACHE" -cf - . | \
        docker exec -i "$container" tar --no-same-owner -xf - -C /tmp/rootfs-diff-buildroot/dl
fi
docker exec -e BUILDROOT_JOBS="${BUILDROOT_JOBS:-2}" "$container" \
    sh /tmp/rootfs-diff-buildroot/recipe/build.sh 2>&1 | tee "$out/build.log"
docker cp "$container:/tmp/rootfs-diff-buildroot/artifacts/." "$out/"
cli=$out/rootfs-diff
"$cli" --format json "$out/before.tar" "$out/after.tar" > "$out/report.json"
"$cli" --format json "$out/before.tar" "$out/after.tar" > "$out/repeat.json"
cmp "$out/report.json" "$out/repeat.json"
"$cli" --format json --fail-on-change "$out/before.tar" "$out/before.tar" > "$out/same.json"
set +e
"$cli" --format json --policy "$root/examples/buildroot/policy.json" \
    "$out/before.tar" "$out/after.tar" > "$out/gated.json"
gate_exit=$?
"$cli" --fail-on-change "$out/before.tar" "$out/after.tar" > "$out/fail-on-change.txt"
change_exit=$?
set -e
if [ "$gate_exit" -ne 1 ] || [ "$change_exit" -ne 1 ]; then
    printf 'Expected exit 1 for policy and fail-on-change gates, got %s/%s\n' "$gate_exit" "$change_exit" >&2
    exit 1
fi
python3 "$root/examples/buildroot/check.py" "$out/before.tar" "$out/after.tar" \
    "$out/report.json" "$out/same.json" "$out/gated.json" > "$out/inspection.json"
"$cli" --format text "$out/before.tar" "$out/after.tar" | tee "$out/report.txt"
python3 - "$out" "$commit" "$image" <<'PY'
import hashlib, pathlib, sys
out = pathlib.Path(sys.argv[1])
print('rootfs-diff source commit: ' + sys.argv[2])
print('Builder: ' + sys.argv[3])
for name in ('before.tar', 'after.tar'):
    with (out / name).open('rb') as source:
        print(name + ' SHA-256: ' + hashlib.file_digest(source, 'sha256').hexdigest())
PY
