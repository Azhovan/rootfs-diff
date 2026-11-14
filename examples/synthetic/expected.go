package synthetic

import rd "github.com/Azhovan/rootfs-diff"

// These expectations are authored from the fixture specification, not produced
// by Scan, Compare, Evaluate, or the CLI. Digests were independently computed
// over the literal payloads. Counts deliberately exclude hardlink payloads.
const (
	startupHash = "46210dddc66714c3d8d226711510cf8421774214016c508c72a833a05370f6b5"
	agentHash   = "d20bc21bb3c7736d8d03ade3ddb4c68b665cdfbca6f6df0f7fdd192f37f59060"
	oldConfig   = "f1bb1d2f3fbdac0e4c9b4620f9da5ca175f0dc9956ba4f41ab9e1f351584dbb5"
	newConfig   = "acc88c88fc4c5cf9d2ce1a7fad631a75a58f79c0e2e23279cf7b9f1c1a973ce8"
	oldPayload  = "6667b2d1aab6a00caa5aee5af8ad9f1465e567abf1c209d15727d57b3e8f6e5f"
	newPayload  = "c24df9d594e949338cd177e47b55eb08c95e2f29b27556ab346b45d642341be3"
	logHash     = "dc51b8c96c2d745df3bd5590d990230a482fd247123599548e0632fdbf97fc22"
)

func fileEntry(path string, mode uint32, size int64, hash string) rd.Entry {
	return rd.Entry{Path: path, Kind: rd.KindRegular, Mode: mode, Size: size, SHA256: hash}
}

func payloadEntry(path string, kind rd.EntryKind, target string, size int64, hash string) rd.Entry {
	return rd.Entry{Path: path, Kind: kind, Mode: 0640, UID: 12, GID: 34, Size: size, SHA256: hash, LinkTarget: target}
}

func beforeManifest() rd.Manifest {
	return rd.Manifest{Entries: []rd.Entry{
		{Path: "etc", Kind: rd.KindDirectory, Mode: 0755},
		{Path: "etc/current", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "/usr/share/old"},
		fileEntry("etc/demo.conf", 0644, 7, oldConfig),
		fileEntry("etc/init.d/S90demo", 0755, 6, startupHash),
		{Path: "usr", Kind: rd.KindDirectory, Mode: 0755},
		fileEntry("usr/bin/demo-agent", 0755, 6, agentHash),
		{Path: "usr/share", Kind: rd.KindDirectory, Mode: 0755},
		payloadEntry("usr/share/alias", rd.KindHardlink, "usr/share/payload", 5, oldPayload),
		payloadEntry("usr/share/chain", rd.KindHardlink, "usr/share/alias", 5, oldPayload),
		payloadEntry("usr/share/payload", rd.KindRegular, "", 5, oldPayload),
		fileEntry("var/log/boot.log", 0600, 3, logHash),
	}}
}

func afterManifest() rd.Manifest {
	return rd.Manifest{Entries: []rd.Entry{
		{Path: "etc", Kind: rd.KindDirectory, Mode: 0755},
		{Path: "etc/current", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../usr/share/new"},
		fileEntry("etc/demo.conf", 0644, 7, newConfig),
		{Path: "usr", Kind: rd.KindDirectory, Mode: 0755},
		fileEntry("usr/bin/demo-agent", 04755, 6, agentHash),
		{Path: "usr/share", Kind: rd.KindDirectory, Mode: 0755},
		payloadEntry("usr/share/alias", rd.KindHardlink, "usr/share/payload", 15, newPayload),
		payloadEntry("usr/share/chain", rd.KindHardlink, "usr/share/payload", 15, newPayload),
		payloadEntry("usr/share/payload", rd.KindRegular, "", 15, newPayload),
		fileEntry("var/log/boot.log", 0600, 3, logHash),
	}}
}

func exclusions() []string {
	return []string{"acls", "directory_sizes", "file_capabilities", "selinux_labels", "timestamps", "unrecognized_pax_metadata", "user_group_names", "xattrs"}
}

func changedReport() rd.Report {
	b, a := beforeManifest().Entries, afterManifest().Entries
	return rd.Report{
		SchemaVersion: 1,
		Summary: rd.Summary{BeforeEntries: 11, AfterEntries: 10, Removed: 1, Modified: 6, Unchanged: 4,
			BeforePayloadBytes: 27, AfterPayloadBytes: 31, NetGrowthBytes: 4},
		Changes: []rd.Change{
			{Path: "etc/current", Kind: rd.ChangeModified, Before: &b[1], After: &a[1], ChangedFields: []rd.Field{rd.FieldLinkTarget}},
			{Path: "etc/demo.conf", Kind: rd.ChangeModified, Before: &b[2], After: &a[2], ChangedFields: []rd.Field{rd.FieldSHA256}},
			{Path: "etc/init.d/S90demo", Kind: rd.ChangeRemoved, Before: &b[3]},
			{Path: "usr/bin/demo-agent", Kind: rd.ChangeModified, Before: &b[5], After: &a[4], ChangedFields: []rd.Field{rd.FieldMode}},
			{Path: "usr/share/alias", Kind: rd.ChangeModified, Before: &b[7], After: &a[6], ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256}},
			{Path: "usr/share/chain", Kind: rd.ChangeModified, Before: &b[8], After: &a[7], ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256, rd.FieldLinkTarget}},
			{Path: "usr/share/payload", Kind: rd.ChangeModified, Before: &b[9], After: &a[8], ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256}},
		},
		PositiveGrowth:     []rd.Growth{{Path: "usr/share/payload", Bytes: 10}},
		MetadataExclusions: exclusions(),
	}
}

func sameReport(candidate bool) rd.Report {
	entries, payload := int64(11), int64(27)
	if candidate {
		entries, payload = 10, 31
	}
	return rd.Report{SchemaVersion: 1, Summary: rd.Summary{BeforeEntries: entries, AfterEntries: entries, Unchanged: entries,
		BeforePayloadBytes: payload, AfterPayloadBytes: payload}, MetadataExclusions: exclusions()}
}

func failingReport() rd.Report {
	r := changedReport()
	zero, four := int64(0), int64(4)
	r.Violations = []rd.Violation{
		{Rule: rd.RuleMaxTotalGrowthBytes, Limit: &zero, Actual: &four},
		{Rule: rd.RuleRequiredPath, Path: "etc/init.d/S90demo"},
	}
	return r
}

func exactIgnoredReport() rd.Report {
	r := changedReport()
	r.Summary = rd.Summary{BeforeEntries: 10, AfterEntries: 9, Removed: 1, Modified: 5, Unchanged: 4,
		BeforePayloadBytes: 20, AfterPayloadBytes: 24, NetGrowthBytes: 4}
	r.Changes = []rd.Change{r.Changes[0], r.Changes[2], r.Changes[3], r.Changes[4], r.Changes[5], r.Changes[6]}
	return r
}

func prefixIgnoredReport() rd.Report {
	r := changedReport()
	r.Summary = rd.Summary{BeforeEntries: 7, AfterEntries: 6, Removed: 1, Modified: 3, Unchanged: 3,
		BeforePayloadBytes: 22, AfterPayloadBytes: 16, NetGrowthBytes: -6}
	r.Changes = r.Changes[:4]
	r.PositiveGrowth = nil
	r.Violations = []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "etc/init.d/S90demo"}}
	return r
}

func allIgnoredReport(required bool) rd.Report {
	r := rd.Report{SchemaVersion: 1, Summary: rd.Summary{BeforeEntries: 3, AfterEntries: 3, Unchanged: 3,
		BeforePayloadBytes: 3, AfterPayloadBytes: 3}, MetadataExclusions: exclusions()}
	if required {
		r.Violations = []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "etc/init.d/S90demo"}}
	}
	return r
}
