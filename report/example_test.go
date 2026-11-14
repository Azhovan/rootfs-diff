package report_test

import (
	"bytes"
	"encoding/json"
	"fmt"

	rd "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/report"
)

func ExampleWrite() {
	before := rd.Manifest{}
	after := rd.Manifest{Entries: []rd.Entry{{
		Path: "bin/current", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../lib/agent",
	}}}
	diff, err := rd.Compare(before, after, rd.CompareOptions{})
	if err != nil {
		panic(err)
	}
	model := rd.Report{
		SchemaVersion: rd.SchemaVersion, Summary: diff.Summary,
		Changes: diff.Changes, PositiveGrowth: diff.PositiveGrowth,
		Warnings: diff.Warnings, MetadataExclusions: rd.MetadataExclusions(),
	}
	// An application can attach Evaluate's violations before rendering.
	for _, format := range []report.Format{report.Text, report.JSON, report.Markdown} {
		var out bytes.Buffer // A file or any caller-owned io.Writer works too.
		if err := report.Write(&out, model, format); err != nil {
			// A writer error can leave a partial report; do not consume it.
			panic(err)
		}
		if format == report.JSON {
			var decoded rd.Report
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				panic(err)
			}
			fmt.Printf("schema %d: %d added, %d payload bytes\n", decoded.SchemaVersion,
				decoded.Summary.Added, decoded.Summary.AfterPayloadBytes)
		} else {
			fmt.Printf("%s report rendered\n", format)
		}
	}
	// Output:
	// text report rendered
	// schema 1: 1 added, 0 payload bytes
	// markdown report rendered
}
