#!/usr/bin/env python3
"""Independently check this Buildroot demo's tar artifacts and JSON reports.

Only Python's standard library is used. Archive payloads are read and hashed,
never extracted or executed. This checks the known demo, not arbitrary input
validity or rootfs-diff's parser rejection behavior.
"""

import argparse
import hashlib
import json
import sys
import tarfile
from pathlib import Path


STARTUP = "etc/init.d/S40network"
CONFIG = "etc/device.conf"
AGENT = "usr/bin/device-agent"
LIBRARY = "usr/lib/libexample.so"
AGENT_V1 = "usr/bin/agent-v1"
AGENT_V2 = "usr/bin/agent-v2"
CONTROLLED_PATHS = {STARTUP, CONFIG, AGENT, LIBRARY}
FIXTURE_PATHS = CONTROLLED_PATHS | {AGENT_V1, AGENT_V2}
EXCLUSIONS = [
    "acls", "directory_sizes", "file_capabilities", "selinux_labels",
    "timestamps", "unrecognized_pax_metadata", "user_group_names", "xattrs",
]
STANDARD_PAX = {
    "path", "linkpath", "size", "uid", "gid", "mtime", "atime", "ctime",
    "uname", "gname",
}


class CheckError(Exception):
    pass


def expect(actual, expected, label):
    """Compare exact JSON types as well as values (True is not integer 1)."""
    if type(actual) is not type(expected):
        raise CheckError(f"{label}: expected {type(expected).__name__}, "
                         f"got {type(actual).__name__}")
    if isinstance(expected, dict):
        if actual.keys() != expected.keys():
            raise CheckError(f"{label}: unexpected/missing keys; "
                             f"expected {sorted(expected)}, got {sorted(actual)}")
        for key in expected:
            expect(actual[key], expected[key], f"{label}.{key}")
    elif isinstance(expected, list):
        if len(actual) != len(expected):
            raise CheckError(f"{label}: expected {len(expected)} items, "
                             f"got {len(actual)}")
        for index, (got, want) in enumerate(zip(actual, expected)):
            expect(got, want, f"{label}[{index}]")
    elif actual != expected:
        raise CheckError(f"{label}: expected {expected!r}, got {actual!r}")


def canonical_path(name):
    if name.startswith("/") or ".." in name.split("/"):
        raise CheckError(f"demo archive has an unsafe path: {name!r}")
    return "/".join(part for part in name.split("/") if part not in ("", "."))


def sha256_stream(stream):
    digest = hashlib.sha256()
    size = 0
    while True:
        chunk = stream.read(64 * 1024)
        if not chunk:
            return size, digest.hexdigest()
        size += len(chunk)
        digest.update(chunk)


def inspect_archive(path):
    entries = {}
    warnings = []
    with tarfile.open(path, mode="r:*", encoding="utf-8", errors="strict") as archive:
        for member in archive:
            name = canonical_path(member.name)
            if member.sparse is not None or any(
                "sparse" in key.lower() for key in member.pax_headers
            ):
                raise CheckError(f"unexpected sparse demo entry: {member.name!r}")
            for key in member.pax_headers:
                if key not in STANDARD_PAX:
                    warnings.append({"path": name or ".", "code": "uncompared_metadata",
                                     "key": key})
            if not name:
                if not member.isdir():
                    raise CheckError("demo archive root record must be a directory")
                continue
            if name in entries:
                raise CheckError(f"duplicate demo archive path: {name!r}")
            entry = {
                "path": name, "kind": "", "mode": member.mode & 0o7777,
                "uid": member.uid, "gid": member.gid, "size": 0, "sha256": "",
                "link_target": "", "device_major": 0, "device_minor": 0,
            }
            if member.isreg():
                entry["kind"] = "regular"
                with archive.extractfile(member) as payload:
                    entry["size"], entry["sha256"] = sha256_stream(payload)
                expect(entry["size"], member.size, f"{name} tar payload size")
            else:
                if member.size != 0:
                    raise CheckError(f"unexpected non-regular payload: {name!r}")
                if member.isdir():
                    entry["kind"] = "directory"
                elif member.issym():
                    entry["kind"] = "symlink"
                    entry["link_target"] = member.linkname
                elif member.islnk():
                    entry["kind"] = "hardlink"
                    entry["link_target"] = canonical_path(member.linkname)
                elif member.ischr() or member.isblk():
                    entry["kind"] = "char_device" if member.ischr() else "block_device"
                    entry["device_major"] = member.devmajor
                    entry["device_minor"] = member.devminor
                elif member.isfifo():
                    entry["kind"] = "fifo"
                else:
                    raise CheckError(f"unexpected demo tar type at {name!r}")
            entries[name] = entry
        if archive.pax_headers:
            raise CheckError("unexpected global PAX metadata in demo archive")

    # Hash only actual regular payloads. Resolve hardlink metadata in memory;
    # tarfile.extractfile is never called for a link or another special entry.
    for name, entry in entries.items():
        if entry["kind"] != "hardlink":
            continue
        terminal = entry
        seen = set()
        while terminal["kind"] == "hardlink":
            target = terminal["link_target"]
            if target in seen or target not in entries:
                raise CheckError(f"invalid hardlink in demo archive: {name!r}")
            seen.add(target)
            terminal = entries[target]
        if terminal["kind"] != "regular":
            raise CheckError(f"non-regular hardlink endpoint: {name!r}")
        for key in ("mode", "uid", "gid", "size", "sha256"):
            entry[key] = terminal[key]
    return entries, warnings


def fixture_entry(path, mode, payload=None, target=None):
    return {
        "path": path, "kind": "symlink" if target is not None else "regular",
        "mode": mode, "uid": 0, "gid": 0,
        "size": len(payload) if payload is not None else 0,
        "sha256": hashlib.sha256(payload).hexdigest() if payload is not None else "",
        "link_target": target if target is not None else "",
        "device_major": 0, "device_minor": 0,
    }


def controlled_entries():
    endpoints = {
        AGENT_V1: fixture_entry(AGENT_V1, 0o755, b"#!/bin/sh\n# demo agent-v1\nexit 0\n"),
        AGENT_V2: fixture_entry(AGENT_V2, 0o755, b"#!/bin/sh\n# demo agent-v2\nexit 0\n"),
    }
    before = {
        **endpoints,
        STARTUP: fixture_entry(STARTUP, 0o755, b"#!/bin/sh\nexit 0\n"),
        CONFIG: fixture_entry(CONFIG, 0o640, b"device=demo\n"),
        AGENT: fixture_entry(AGENT, 0o777, target="agent-v1"),
        LIBRARY: fixture_entry(LIBRARY, 0o644, b"\0" * 1024),
    }
    after = {
        **endpoints,
        CONFIG: fixture_entry(CONFIG, 0o666, b"device=demo\n"),
        AGENT: fixture_entry(AGENT, 0o777, target="agent-v2"),
        LIBRARY: fixture_entry(LIBRARY, 0o644, b"\0" * (421 * 1024)),
    }
    return before, after


def report_expected(before, after, warnings, same=False, gated=False):
    changes = []
    if not same:
        fields = {CONFIG: ["mode"], AGENT: ["link_target"],
                  LIBRARY: ["size", "sha256"]}
        for path in sorted(CONTROLLED_PATHS):
            changes.append({
                "path": path, "kind": "removed" if path == STARTUP else "modified",
                "before": before[path], "after": after.get(path),
                "changed_fields": [] if path == STARTUP else fields[path],
            })
    before_bytes = sum(e["size"] for e in before.values() if e["kind"] == "regular")
    after_bytes = sum(e["size"] for e in after.values() if e["kind"] == "regular")
    return {
        "schema_version": 1,
        "summary": {
            "before_entries": len(before), "after_entries": len(after),
            "added": 0, "removed": 0 if same else 1, "modified": 0 if same else 3,
            "unchanged": len(before) if same else len(before) - 4,
            "before_payload_bytes": before_bytes, "after_payload_bytes": after_bytes,
            "net_growth_bytes": after_bytes - before_bytes,
        },
        "changes": changes,
        "positive_growth": [] if same else [{"path": LIBRARY, "bytes": 430080}],
        "violations": [
            {"rule": "max_total_growth_bytes", "path": "", "limit": 409600, "actual": 430063},
            {"rule": "required_path", "path": STARTUP, "limit": None, "actual": None},
        ] if gated else [],
        "warnings": warnings, "metadata_exclusions": EXCLUSIONS,
    }


def sourced_warnings(before, after):
    return sorted(
        [dict(warning, source=source) for source, items in
         (("before", before), ("after", after)) for warning in items],
        key=lambda warning: (warning["path"], warning["code"], warning["key"], warning["source"]),
    )


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise CheckError(f"duplicate JSON key: {key!r}")
        result[key] = value
    return result


def reject_number(value):
    raise CheckError(f"unexpected non-integer JSON number: {value}")


def check_report(path, expected):
    actual = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique_object,
                        parse_float=reject_number, parse_constant=reject_number)
    expect(actual, expected, path.name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("before", type=Path)
    parser.add_argument("after", type=Path)
    parser.add_argument("report", type=Path)
    parser.add_argument("same", type=Path, nargs="?")
    parser.add_argument("gated", type=Path, nargs="?")
    args = parser.parse_args()
    if (args.same is None) != (args.gated is None):
        parser.error("provide both SAME.json (before versus itself) and GATED.json")
    before, before_warnings = inspect_archive(args.before)
    after, after_warnings = inspect_archive(args.after)
    expected_before, expected_after = controlled_entries()
    expect({p: e for p, e in before.items() if p in FIXTURE_PATHS},
           expected_before, "fixture before archive entries and agent endpoints")
    expect({p: e for p, e in after.items() if p in FIXTURE_PATHS},
           expected_after, "fixture after archive entries and agent endpoints")
    expect({p: e for p, e in after.items() if p not in CONTROLLED_PATHS},
           {p: e for p, e in before.items() if p not in CONTROLLED_PATHS},
           "all other compared archive entries")
    warnings = sourced_warnings(before_warnings, after_warnings)
    expected = report_expected(before, after, warnings)
    expect(expected["summary"]["net_growth_bytes"], 430063, "independent net growth")
    check_report(args.report, expected)
    if args.same is not None:
        check_report(args.same, report_expected(
            before, before, sourced_warnings(before_warnings, before_warnings), same=True))
        check_report(args.gated, report_expected(before, after, warnings, gated=True))
    with args.before.open("rb") as source:
        _, before_sha256 = sha256_stream(source)
    with args.after.open("rb") as source:
        _, after_sha256 = sha256_stream(source)
    print(json.dumps({
        "artifact_sha256": {"before": before_sha256, "after": after_sha256},
        "summary": expected["summary"], "controlled_changes": expected["changes"],
        "positive_growth": expected["positive_growth"], "warnings": warnings,
        "same_and_gated_reports_checked": args.same is not None,
    }, sort_keys=True, separators=(",", ":")))


if __name__ == "__main__":
    try:
        main()
    except (CheckError, OSError, tarfile.TarError, ValueError) as error:
        print(f"Buildroot proof check failed: {error}", file=sys.stderr)
        sys.exit(1)
