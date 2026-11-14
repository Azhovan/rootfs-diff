package rootfsdiff_test

import (
	"errors"
	"fmt"
	"strings"

	rd "github.com/Azhovan/rootfs-diff"
)

func ExampleParsePolicy() {
	policy, err := rd.ParsePolicy(strings.NewReader(`{
		"schema_version": 1,
		"required_paths": ["etc/init.d/S40network", "usr/bin/device-agent"],
		"max_total_growth_bytes": 1048576
	}`))
	if err != nil {
		panic(err)
	}
	fmt.Println(policy.SchemaVersion, policy.RequiredPaths)
	fmt.Printf("maximum net growth: %d bytes\n", *policy.MaxTotalGrowthBytes)

	_, err = rd.ParsePolicy(strings.NewReader(`{"schema_version":1,"required_paths":[],"typo":0}`))
	fmt.Println("invalid policy:", errors.Is(err, rd.ErrInvalidInput))
	// Output:
	// 1 [etc/init.d/S40network usr/bin/device-agent]
	// maximum net growth: 1048576 bytes
	// invalid policy: true
}

func ExampleEvaluate() {
	// These manifests contain only explicit entries. An implicit "etc" parent
	// is not required here; a symlink's presence is enough, without resolving it.
	before := rd.Manifest{Entries: []rd.Entry{{Path: "obsolete", Kind: rd.KindDirectory}}}
	after := rd.Manifest{Entries: []rd.Entry{
		{Path: "etc/device.conf", Kind: rd.KindSymlink, LinkTarget: "../settings"},
		{Path: "var/log/boot", Kind: rd.KindFIFO},
	}}
	diff, err := rd.Compare(before, after, rd.CompareOptions{IgnorePrefixes: []string{"var/log"}})
	if err != nil {
		panic(err)
	}
	policy, err := rd.ParsePolicy(strings.NewReader(`{
		"schema_version": 1,
		"required_paths": ["var/log/boot", "usr/bin/device-agent", "etc/device.conf"],
		"max_total_growth_bytes": 0
	}`))
	if err != nil {
		panic(err)
	}
	violations, err := rd.Evaluate(after, diff, policy)
	if err != nil {
		panic(err)
	}
	// Ignored var/log/boot still satisfies presence. The symlink also passes.
	// Added/removed nonregular entries have zero payload growth, so zero passes.
	fmt.Printf("net growth: %d bytes\n", diff.Summary.NetGrowthBytes)
	for _, violation := range violations {
		fmt.Println(violation.Rule, violation.Path)
	}
	// Output:
	// net growth: 0 bytes
	// required_path usr/bin/device-agent
}
