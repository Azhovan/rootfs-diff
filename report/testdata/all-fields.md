# rootfs-diff report (schema 1)

## Summary

- Entries: before 6, after 6
- Changes: added 1, removed 1, modified 5, unchanged 0
- Regular payload: before 12 bytes, after 6 bytes, net -6 bytes

Payload totals count retained regular entries only, excluding hardlink aliases; they are not filesystem or flash usage.

## Positive growth (showing 0 of 0)

- None.

## Changes (7)

- modified <code>&#34;block&#34;</code>
  - Changed fields: kind, mode, uid, gid, device_major, device_minor
  - Before: kind=char_device mode=0600 uid=-2 gid=4 device_major=1 device_minor=3
  - After: kind=block_device mode=01640 uid=3 gid=5 device_major=8 device_minor=9
- added <code>&#34;dir&#34;</code>
  - Before: absent
  - After: kind=directory mode=01777 uid=10 gid=20
- removed <code>&#34;fifo&#34;</code>
  - Before: kind=fifo mode=0660 uid=30 gid=40
  - After: absent
- modified <code>&#34;hard&#34;</code>
  - Changed fields: mode, uid, gid, size, sha256, link_target
  - Before: kind=hardlink mode=0644 uid=1 gid=2 size=5 sha256=<code>&#34;aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&#34;</code> link_target=<code>&#34;old&#34;</code>
  - After: kind=hardlink mode=06750 uid=-1 gid=-2 size=8 sha256=<code>&#34;bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&#34;</code> link_target=<code>&#34;new&#34;</code>
- modified <code>&#34;reg&#34;</code>
  - Changed fields: size, sha256
  - Before: kind=regular mode=0644 uid=0 gid=0 size=10 sha256=<code>&#34;aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&#34;</code>
  - After: kind=regular mode=0644 uid=0 gid=0 size=6 sha256=<code>&#34;bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&#34;</code>
- modified <code>&#34;sym&#34;</code>
  - Changed fields: link_target
  - Before: kind=symlink mode=0777 uid=0 gid=0 link_target=<code>&#34;old&#34;</code>
  - After: kind=symlink mode=0777 uid=0 gid=0 link_target=<code>&#34;new&#34;</code>
- modified <code>&#34;type&#34;</code>
  - Changed fields: kind, mode, size, sha256, link_target
  - Before: kind=regular mode=0644 uid=0 gid=0 size=2 sha256=<code>&#34;aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&#34;</code>
  - After: kind=symlink mode=0777 uid=0 gid=0 link_target=<code>&#34;&#46;&#46;&#47;reg&#34;</code>

## Policy violations (0)

- None recorded.

## Warnings (0)

- None recorded.

## Metadata exclusions

- acls, directory_sizes, file_capabilities, selinux_labels, timestamps, unrecognized_pax_metadata, user_group_names, xattrs

These metadata are not compared. No changes does not establish firmware equivalence, bootability, compatibility, or security.
