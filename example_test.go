package rootfsdiff_test

import (
	"encoding/json"
	"fmt"

	rd "github.com/Azhovan/rootfs-diff"
)

func ExampleManifest() {
	manifest := rd.Manifest{Entries: []rd.Entry{{
		Path: "etc/device.conf", Kind: rd.KindRegular, Mode: 0640,
		UID: 123, GID: 456, Size: 0,
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}}}
	if err := manifest.Validate(); err != nil {
		panic(err)
	}
	fmt.Printf("%s mode=%04o uid=%d gid=%d\n", manifest.Entries[0].Path,
		manifest.Entries[0].Mode, manifest.Entries[0].UID, manifest.Entries[0].GID)
	// Output: etc/device.conf mode=0640 uid=123 gid=456
}

func ExampleReport() {
	report := rd.Report{SchemaVersion: rd.SchemaVersion, MetadataExclusions: rd.MetadataExclusions()}
	if err := report.Validate(); err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
	// Output: {"schema_version":1,"summary":{"before_entries":0,"after_entries":0,"added":0,"removed":0,"modified":0,"unchanged":0,"before_payload_bytes":0,"after_payload_bytes":0,"net_growth_bytes":0},"changes":[],"positive_growth":[],"violations":[],"warnings":[],"metadata_exclusions":["acls","directory_sizes","file_capabilities","selinux_labels","timestamps","unrecognized_pax_metadata","user_group_names","xattrs"]}
}
