# rootfs-diff report (schema 1)

## Summary

- Entries: before 3, after 3
- Changes: added 1, removed 1, modified 1, unchanged 1
- Regular payload: before 10 bytes, after 7 bytes, net -3 bytes

Payload totals count retained regular entries only, excluding hardlink aliases; they are not filesystem or flash usage.

## Positive growth (showing 1 of 1)

- <code>&#34;etc&#47;new&#34;</code>: +2 bytes

## Changes (3)

- modified <code>&#34;etc&#47;device&#46;conf&#34;</code>
  - Changed fields: mode, size, sha256
  - Before: kind=regular mode=06750 uid=0 gid=0 size=4 sha256=<code>&#34;7aa7a5359173d05b63cfd682e3c38487f3cb4f7f1d60659fe59fab1505977d4c&#34;</code>
  - After: kind=regular mode=0640 uid=0 gid=0 size=2 sha256=<code>&#34;8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4&#34;</code>
- added <code>&#34;etc&#47;new&#34;</code>
  - Before: absent
  - After: kind=regular mode=0644 uid=0 gid=0 size=2 sha256=<code>&#34;2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df&#34;</code>
- removed <code>&#34;obsolete&#34;</code>
  - Before: kind=regular mode=0644 uid=0 gid=0 size=3 sha256=<code>&#34;ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad&#34;</code>
  - After: absent

## Policy violations (1)

- required_path: missing explicit candidate path <code>&#34;usr&#47;bin&#47;agent&#34;</code>

## Warnings (2)

- after <code>&#34;etc&#47;device&#46;conf&#34;</code>: uncompared_metadata key <code>&#34;SCHILY&#46;xattr&#46;security&#46;capability&#34;</code>
- before <code>&#34;etc&#47;device&#46;conf&#34;</code>: uncompared_metadata key <code>&#34;SCHILY&#46;xattr&#46;security&#46;capability&#34;</code>

## Metadata exclusions

- acls, directory_sizes, file_capabilities, selinux_labels, timestamps, unrecognized_pax_metadata, user_group_names, xattrs

These metadata are not compared. No changes does not establish firmware equivalence, bootability, compatibility, or security.
