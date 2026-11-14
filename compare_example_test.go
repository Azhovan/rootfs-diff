package rootfsdiff_test

import (
	"crypto/sha256"
	"fmt"

	rd "github.com/Azhovan/rootfs-diff"
)

func ExampleCompare() {
	file := func(path, contents string) rd.Entry {
		return rd.Entry{
			Path: path, Kind: rd.KindRegular, Mode: 0644,
			Size: int64(len(contents)), SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(contents))),
		}
	}
	// Caller-built manifests may be unordered. No implicit directories are needed.
	before := rd.Manifest{Entries: []rd.Entry{
		file("usr/bin/agent", "v1"),
		file("var/log/boot", "old log"),
		file("etc/device.conf", "enabled\n"),
	}}
	after := rd.Manifest{
		Entries: []rd.Entry{
			file("usr/bin/agent", "version2"),
			file("etc/new", "x"),
			file("var/log/boot", "new log contents"),
			file("etc/device.conf", "enabled\n"),
		},
		Warnings: []rd.Warning{{
			Path: "var/log/boot", Code: rd.WarningUncomparedMetadata, Key: "SCHILY.xattr.user.build",
		}},
	}
	diff, err := rd.Compare(before, after, rd.CompareOptions{IgnorePrefixes: []string{"var/log"}})
	if err != nil {
		panic(err)
	}
	for _, change := range diff.Changes {
		fmt.Printf("%s %s %v\n", change.Kind, change.Path, change.ChangedFields)
	}
	// Totals count retained regular payloads, not filesystem disk usage.
	fmt.Printf("payload: %d -> %d bytes (%+d)\n", diff.Summary.BeforePayloadBytes,
		diff.Summary.AfterPayloadBytes, diff.Summary.NetGrowthBytes)
	for _, growth := range diff.PositiveGrowth {
		fmt.Printf("growth: %s +%d bytes\n", growth.Path, growth.Bytes)
	}
	// Ignoring changes and payload at a path never suppresses its warnings.
	for _, warning := range diff.Warnings {
		fmt.Printf("warning: %s %s %s\n", warning.Source, warning.Path, warning.Key)
	}
	// Output:
	// added etc/new []
	// modified usr/bin/agent [size sha256]
	// payload: 10 -> 17 bytes (+7)
	// growth: usr/bin/agent +6 bytes
	// growth: etc/new +1 bytes
	// warning: after var/log/boot SCHILY.xattr.user.build
}
