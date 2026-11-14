package rootfsdiff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	rd "github.com/Azhovan/rootfs-diff"
)

const compareDigestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const compareDigestB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func compareFile(path string, size int64) rd.Entry {
	return rd.Entry{Path: path, Kind: rd.KindRegular, Mode: 0644, Size: size, SHA256: compareDigestA}
}

func compareLink(path, target string, terminal rd.Entry) rd.Entry {
	terminal.Path, terminal.Kind, terminal.LinkTarget = path, rd.KindHardlink, target
	return terminal
}

func requireComparison(t *testing.T, before, after rd.Manifest, opts rd.CompareOptions) rd.Diff {
	t.Helper()
	d, err := rd.Compare(before, after, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("Compare produced an invalid diff: %v", err)
	}
	return d
}

func compareCheck(t *testing.T, got, want rd.Diff) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("comparison mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestCompareEmptyAndUnchanged(t *testing.T) {
	for _, m := range []rd.Manifest{{}, {Entries: []rd.Entry{}, Warnings: []rd.Warning{}}} {
		d := requireComparison(t, m, m, rd.CompareOptions{})
		got, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"summary":{"before_entries":0,"after_entries":0,"added":0,"removed":0,"modified":0,"unchanged":0,"before_payload_bytes":0,"after_payload_bytes":0,"net_growth_bytes":0},"changes":[],"positive_growth":[],"warnings":[]}`
		if string(got) != want {
			t.Fatalf("empty diff = %s, want %s", got, want)
		}
	}
	m := rd.Manifest{Entries: []rd.Entry{compareFile("z", 3), {Path: "a", Kind: rd.KindDirectory, Mode: 01777}}}
	d := requireComparison(t, m, m, rd.CompareOptions{})
	compareCheck(t, d, rd.Diff{
		Summary: rd.Summary{BeforeEntries: 2, AfterEntries: 2, Unchanged: 2, BeforePayloadBytes: 3, AfterPayloadBytes: 3},
		Changes: []rd.Change{}, PositiveGrowth: []rd.Growth{}, Warnings: []rd.Warning{},
	})
}

func TestCompareIndependentFields(t *testing.T) {
	file := compareFile("file", 5)
	link := rd.Entry{Path: "file", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../original"}
	device := rd.Entry{Path: "file", Kind: rd.KindCharDevice, Mode: 0600, DeviceMajor: 1, DeviceMinor: 2}
	for _, tc := range []struct {
		name                            string
		before                          rd.Entry
		change                          func(*rd.Entry)
		fields                          []rd.Field
		beforeBytes, afterBytes, growth int64
	}{
		{"kind", rd.Entry{Path: "file", Kind: rd.KindDirectory}, func(e *rd.Entry) { e.Kind = rd.KindFIFO }, []rd.Field{rd.FieldKind}, 0, 0, 0},
		{"special mode bits", file, func(e *rd.Entry) { e.Mode = 06754 }, []rd.Field{rd.FieldMode}, 5, 5, 0},
		{"uid", file, func(e *rd.Entry) { e.UID = -12 }, []rd.Field{rd.FieldUID}, 5, 5, 0},
		{"gid", file, func(e *rd.Entry) { e.GID = 34 }, []rd.Field{rd.FieldGID}, 5, 5, 0},
		{"size alone", file, func(e *rd.Entry) { e.Size = 9 }, []rd.Field{rd.FieldSize}, 5, 9, 4},
		{"equal size content", file, func(e *rd.Entry) { e.SHA256 = compareDigestB }, []rd.Field{rd.FieldSHA256}, 5, 5, 0},
		{"symlink target verbatim", link, func(e *rd.Entry) { e.LinkTarget = "/absolute/../target" }, []rd.Field{rd.FieldLinkTarget}, 0, 0, 0},
		{"device major", device, func(e *rd.Entry) { e.DeviceMajor = 3 }, []rd.Field{rd.FieldDeviceMajor}, 0, 0, 0},
		{"device minor", device, func(e *rd.Entry) { e.DeviceMinor = 4 }, []rd.Field{rd.FieldDeviceMinor}, 0, 0, 0},
		{"size and content", file, func(e *rd.Entry) { e.Size, e.SHA256 = 9, compareDigestB }, []rd.Field{rd.FieldSize, rd.FieldSHA256}, 5, 9, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := tc.before
			tc.change(&after)
			d := requireComparison(t, rd.Manifest{Entries: []rd.Entry{tc.before}}, rd.Manifest{Entries: []rd.Entry{after}}, rd.CompareOptions{})
			growth := []rd.Growth{}
			if tc.growth != 0 {
				growth = append(growth, rd.Growth{Path: "file", Bytes: tc.growth})
			}
			compareCheck(t, d, rd.Diff{
				Summary: rd.Summary{BeforeEntries: 1, AfterEntries: 1, Modified: 1,
					BeforePayloadBytes: tc.beforeBytes, AfterPayloadBytes: tc.afterBytes, NetGrowthBytes: tc.afterBytes - tc.beforeBytes},
				Changes:        []rd.Change{{Path: "file", Kind: rd.ChangeModified, Before: &tc.before, After: &after, ChangedFields: tc.fields}},
				PositiveGrowth: growth, Warnings: []rd.Warning{},
			})
		})
	}
}

func TestCompareAllFieldsInContractOrder(t *testing.T) {
	terminal := compareFile("terminal", 9)
	terminal.Mode, terminal.UID, terminal.GID = 06751, 100, 200
	old := rd.Entry{Path: "file", Kind: rd.KindBlockDevice, Mode: 0600, UID: 1, GID: 2, DeviceMajor: 3, DeviceMinor: 4}
	next := compareLink("file", "terminal", terminal)
	d := requireComparison(t, rd.Manifest{Entries: []rd.Entry{terminal, old}}, rd.Manifest{Entries: []rd.Entry{next, terminal}}, rd.CompareOptions{})
	compareCheck(t, d, rd.Diff{
		Summary: rd.Summary{BeforeEntries: 2, AfterEntries: 2, Modified: 1, Unchanged: 1, BeforePayloadBytes: 9, AfterPayloadBytes: 9},
		Changes: []rd.Change{{Path: "file", Kind: rd.ChangeModified, Before: &old, After: &next,
			ChangedFields: []rd.Field{rd.FieldKind, rd.FieldMode, rd.FieldUID, rd.FieldGID, rd.FieldSize, rd.FieldSHA256, rd.FieldLinkTarget, rd.FieldDeviceMajor, rd.FieldDeviceMinor}}},
		PositiveGrowth: []rd.Growth{}, Warnings: []rd.Warning{},
	})
}

func TestCompareAddRemoveAndExplicitDirectories(t *testing.T) {
	child := compareFile("dir/child", 2)
	directory := rd.Entry{Path: "dir", Kind: rd.KindDirectory, Mode: 0755}
	for _, add := range []bool{false, true} {
		t.Run(fmt.Sprintf("add=%t", add), func(t *testing.T) {
			before := rd.Manifest{Entries: []rd.Entry{directory, child}}
			after := rd.Manifest{Entries: []rd.Entry{child}}
			want := rd.Diff{
				Summary:        rd.Summary{BeforeEntries: 2, AfterEntries: 1, Removed: 1, Unchanged: 1, BeforePayloadBytes: 2, AfterPayloadBytes: 2},
				Changes:        []rd.Change{{Path: "dir", Kind: rd.ChangeRemoved, Before: &directory, ChangedFields: []rd.Field{}}},
				PositiveGrowth: []rd.Growth{}, Warnings: []rd.Warning{},
			}
			if add {
				before, after = after, before
				want.Summary.BeforeEntries, want.Summary.AfterEntries, want.Summary.Removed, want.Summary.Added = 1, 2, 0, 1
				want.Changes[0] = rd.Change{Path: "dir", Kind: rd.ChangeAdded, After: &directory, ChangedFields: []rd.Field{}}
			}
			compareCheck(t, requireComparison(t, before, after, rd.CompareOptions{}), want)
		})
	}
	removed, added := compareFile("old", 8), compareFile("new", 3)
	d := requireComparison(t, rd.Manifest{Entries: []rd.Entry{removed}}, rd.Manifest{Entries: []rd.Entry{added}}, rd.CompareOptions{})
	compareCheck(t, d, rd.Diff{
		Summary: rd.Summary{BeforeEntries: 1, AfterEntries: 1, Added: 1, Removed: 1, BeforePayloadBytes: 8, AfterPayloadBytes: 3, NetGrowthBytes: -5},
		Changes: []rd.Change{
			{Path: "new", Kind: rd.ChangeAdded, After: &added, ChangedFields: []rd.Field{}},
			{Path: "old", Kind: rd.ChangeRemoved, Before: &removed, ChangedFields: []rd.Field{}},
		},
		PositiveGrowth: []rd.Growth{{Path: "new", Bytes: 3}}, Warnings: []rd.Warning{},
	})
}

func TestCompareHardlinks(t *testing.T) {
	terminal := compareFile("target", 5)
	other := compareFile("other", 5)
	for _, tc := range []struct {
		name                    string
		before, after           []rd.Entry
		wantFields              map[string][]rd.Field
		beforeBytes, afterBytes int64
		growth                  []rd.Growth
	}{
		{
			name:       "target change with same effective data",
			before:     []rd.Entry{terminal, other, compareLink("alias", "target", terminal)},
			after:      []rd.Entry{terminal, other, compareLink("alias", "other", other)},
			wantFields: map[string][]rd.Field{"alias": {rd.FieldLinkTarget}}, beforeBytes: 10, afterBytes: 10,
		},
		{
			name:       "regular becomes hardlink",
			before:     []rd.Entry{terminal, compareFile("alias", 5)},
			after:      []rd.Entry{terminal, compareLink("alias", "target", terminal)},
			wantFields: map[string][]rd.Field{"alias": {rd.FieldKind, rd.FieldLinkTarget}}, beforeBytes: 10, afterBytes: 5,
		},
		{
			name:       "hardlink becomes regular",
			before:     []rd.Entry{terminal, compareLink("alias", "target", terminal)},
			after:      []rd.Entry{terminal, compareFile("alias", 5)},
			wantFields: map[string][]rd.Field{"alias": {rd.FieldKind, rd.FieldLinkTarget}}, beforeBytes: 5, afterBytes: 10,
			growth: []rd.Growth{{Path: "alias", Bytes: 5}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := requireComparison(t, rd.Manifest{Entries: tc.before}, rd.Manifest{Entries: tc.after}, rd.CompareOptions{})
			if d.Summary.BeforePayloadBytes != tc.beforeBytes || d.Summary.AfterPayloadBytes != tc.afterBytes || d.Summary.NetGrowthBytes != tc.afterBytes-tc.beforeBytes {
				t.Fatalf("wrong payload summary: %+v", d.Summary)
			}
			if len(d.Changes) != len(tc.wantFields) || !slices.Equal(d.PositiveGrowth, tc.growth) {
				t.Fatalf("unexpected changes/growth: %+v", d)
			}
			for _, c := range d.Changes {
				if c.Kind != rd.ChangeModified || !slices.Equal(c.ChangedFields, tc.wantFields[c.Path]) {
					t.Fatalf("unexpected change: %+v", c)
				}
			}
		})
	}
	// A forward chain inherits every effective field; all three paths show the
	// same inode change, but only its single regular entry contributes growth.
	next := terminal
	next.Mode, next.UID, next.GID, next.Size, next.SHA256 = 04750, 12, 34, 8, compareDigestB
	before := rd.Manifest{Entries: []rd.Entry{compareLink("a", "b", terminal), compareLink("b", "target", terminal), terminal}}
	after := rd.Manifest{Entries: []rd.Entry{compareLink("b", "target", next), next, compareLink("a", "b", next)}}
	d := requireComparison(t, before, after, rd.CompareOptions{})
	if d.Summary != (rd.Summary{BeforeEntries: 3, AfterEntries: 3, Modified: 3, BeforePayloadBytes: 5, AfterPayloadBytes: 8, NetGrowthBytes: 3}) {
		t.Fatalf("wrong summary: %+v", d.Summary)
	}
	if !reflect.DeepEqual(d.PositiveGrowth, []rd.Growth{{Path: "target", Bytes: 3}}) {
		t.Fatalf("aliases counted as payload growth: %+v", d.PositiveGrowth)
	}
	for i, path := range []string{"a", "b", "target"} {
		if d.Changes[i].Path != path || !slices.Equal(d.Changes[i].ChangedFields, []rd.Field{rd.FieldMode, rd.FieldUID, rd.FieldGID, rd.FieldSize, rd.FieldSHA256}) {
			t.Fatalf("effective change missing at %s: %+v", path, d.Changes)
		}
	}
	// Ignoring the terminal does not invalidate retained aliases, and their
	// effective changes remain visible without contributing payload bytes.
	d = requireComparison(t, before, after, rd.CompareOptions{IgnorePaths: []string{"target"}})
	if d.Summary != (rd.Summary{BeforeEntries: 2, AfterEntries: 2, Modified: 2}) || len(d.PositiveGrowth) != 0 || len(d.Changes) != 2 {
		t.Fatalf("ignored terminal affected alias semantics: %+v", d)
	}
}

func TestComparePayloadKinds(t *testing.T) {
	file := compareFile("regular", 9)
	entries := []rd.Entry{
		file, compareLink("hardlink", "regular", file),
		{Path: "directory", Kind: rd.KindDirectory},
		{Path: "symlink", Kind: rd.KindSymlink, LinkTarget: "regular"},
		{Path: "char", Kind: rd.KindCharDevice, DeviceMajor: 1, DeviceMinor: 2},
		{Path: "block", Kind: rd.KindBlockDevice, DeviceMajor: 3, DeviceMinor: 4},
		{Path: "fifo", Kind: rd.KindFIFO},
	}
	d := requireComparison(t, rd.Manifest{}, rd.Manifest{Entries: entries}, rd.CompareOptions{})
	if d.Summary != (rd.Summary{AfterEntries: 7, Added: 7, AfterPayloadBytes: 9, NetGrowthBytes: 9}) || !reflect.DeepEqual(d.PositiveGrowth, []rd.Growth{{Path: "regular", Bytes: 9}}) {
		t.Fatalf("only regular entries should count: %+v", d)
	}
	d = requireComparison(t, rd.Manifest{Entries: entries}, rd.Manifest{}, rd.CompareOptions{})
	if d.Summary != (rd.Summary{BeforeEntries: 7, Removed: 7, BeforePayloadBytes: 9, NetGrowthBytes: -9}) || len(d.PositiveGrowth) != 0 {
		t.Fatalf("wrong removed-kind accounting: %+v", d)
	}
}

func TestCompareGrowthTransitionsAndRanking(t *testing.T) {
	before := rd.Manifest{Entries: []rd.Entry{
		compareFile("grow", 3), compareFile("shrink", 20), compareFile("removed", 10),
		compareFile("to-link", 7), {Path: "to-regular", Kind: rd.KindSymlink, LinkTarget: "elsewhere"},
		compareFile("empty", 0),
	}}
	after := rd.Manifest{Entries: []rd.Entry{
		compareFile("grow", 8), compareFile("shrink", 4), compareFile("added", 5),
		{Path: "to-link", Kind: rd.KindSymlink, LinkTarget: "elsewhere"}, compareFile("to-regular", 6),
		compareFile("empty", 0),
	}}
	d := requireComparison(t, before, after, rd.CompareOptions{})
	if d.Summary != (rd.Summary{BeforeEntries: 6, AfterEntries: 6, Added: 1, Removed: 1, Modified: 4, Unchanged: 1, BeforePayloadBytes: 40, AfterPayloadBytes: 23, NetGrowthBytes: -17}) {
		t.Fatalf("wrong growth summary: %+v", d.Summary)
	}
	if !reflect.DeepEqual(d.PositiveGrowth, []rd.Growth{{Path: "to-regular", Bytes: 6}, {Path: "added", Bytes: 5}, {Path: "grow", Bytes: 5}}) {
		t.Fatalf("wrong positive growth ordering: %+v", d.PositiveGrowth)
	}
	// The core list is complete even when a human renderer will show only ten.
	var many rd.Manifest
	for i := 12; i >= 0; i-- {
		many.Entries = append(many.Entries, compareFile(fmt.Sprintf("file-%02d", i), 1))
	}
	d = requireComparison(t, rd.Manifest{}, many, rd.CompareOptions{})
	if len(d.PositiveGrowth) != 13 {
		t.Fatalf("growth list truncated to %d", len(d.PositiveGrowth))
	}
	for i, g := range d.PositiveGrowth {
		if g != (rd.Growth{Path: fmt.Sprintf("file-%02d", i), Bytes: 1}) {
			t.Fatalf("growth tie %d: %+v", i, g)
		}
	}
	// Swapping which name stores payload changes topology and growth ranking,
	// while its net payload is unchanged.
	fileA, fileB := compareFile("a", 5), compareFile("b", 5)
	d = requireComparison(t,
		rd.Manifest{Entries: []rd.Entry{fileA, compareLink("b", "a", fileA)}},
		rd.Manifest{Entries: []rd.Entry{compareLink("a", "b", fileB), fileB}}, rd.CompareOptions{})
	if d.Summary.NetGrowthBytes != 0 || d.Summary.Modified != 2 || !reflect.DeepEqual(d.PositiveGrowth, []rd.Growth{{Path: "b", Bytes: 5}}) {
		t.Fatalf("wrong topology accounting: %+v", d)
	}
}

func TestCompareIgnoreBoundaries(t *testing.T) {
	entries := []rd.Entry{
		{Path: "var", Kind: rd.KindDirectory}, {Path: "var/log", Kind: rd.KindDirectory},
		compareFile("var/log/a", 1), compareFile("var/log/deep/b", 2), compareFile("var/logger", 4),
		compareFile("var/logs/a", 8), compareFile("var/logbook/a", 16), compareFile("other", 32),
		compareFile(`var\log`, 64), compareFile("a/*", 128), compareFile("a/literal", 256),
	}
	for _, tc := range []struct {
		name  string
		opts  rd.CompareOptions
		paths []string
		bytes int64
	}{
		{"exact directory only", rd.CompareOptions{IgnorePaths: []string{"var/log"}}, []string{"a/*", "a/literal", "other", "var", "var/log/a", "var/log/deep/b", "var/logbook/a", "var/logger", "var/logs/a", `var\log`}, 511},
		{"exact file only", rd.CompareOptions{IgnorePaths: []string{"var/log/a"}}, []string{"a/*", "a/literal", "other", "var", "var/log", "var/log/deep/b", "var/logbook/a", "var/logger", "var/logs/a", `var\log`}, 510},
		{"prefix boundary", rd.CompareOptions{IgnorePrefixes: []string{"var/log"}}, []string{"a/*", "a/literal", "other", "var", "var/logbook/a", "var/logger", "var/logs/a", `var\log`}, 508},
		{"top-level prefix", rd.CompareOptions{IgnorePrefixes: []string{"var"}}, []string{"a/*", "a/literal", "other", `var\log`}, 480},
		{"literal glob character", rd.CompareOptions{IgnorePaths: []string{"a/*"}}, []string{"a/literal", "other", "var", "var/log", "var/log/a", "var/log/deep/b", "var/logbook/a", "var/logger", "var/logs/a", `var\log`}, 383},
		{"overlap and duplicates", rd.CompareOptions{IgnorePaths: []string{"var/log/a", "other", "other"}, IgnorePrefixes: []string{"var/log", "var/log/deep", "var/log"}}, []string{"a/*", "a/literal", "var", "var/logbook/a", "var/logger", "var/logs/a", `var\log`}, 476},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, removal := range []bool{false, true} {
				before, after := rd.Manifest{}, rd.Manifest{Entries: entries}
				if removal {
					before, after = after, before
				}
				d := requireComparison(t, before, after, tc.opts)
				paths := make([]string, 0, len(d.Changes))
				for _, c := range d.Changes {
					paths = append(paths, c.Path)
				}
				if !slices.Equal(paths, tc.paths) {
					t.Fatalf("removal=%t paths = %q, want %q", removal, paths, tc.paths)
				}
				if removal {
					if d.Summary.BeforePayloadBytes != tc.bytes || d.Summary.AfterPayloadBytes != 0 || d.Summary.NetGrowthBytes != -tc.bytes || d.Summary.Removed != int64(len(tc.paths)) {
						t.Fatalf("wrong removed summary: %+v", d.Summary)
					}
				} else if d.Summary.AfterPayloadBytes != tc.bytes || d.Summary.BeforePayloadBytes != 0 || d.Summary.NetGrowthBytes != tc.bytes || d.Summary.Added != int64(len(tc.paths)) {
					t.Fatalf("wrong added summary: %+v", d.Summary)
				}
			}
		})
	}
	// The same prefix hides a removal, addition and modification on the two
	// sides, while its neighboring name remains visible.
	d := requireComparison(t,
		rd.Manifest{Entries: []rd.Entry{compareFile("var/log/old", 10), compareFile("var/log/same", 20), compareFile("var/logger", 2)}},
		rd.Manifest{Entries: []rd.Entry{compareFile("var/log/new", 30), compareFile("var/log/same", 40), compareFile("var/logger", 3)}},
		rd.CompareOptions{IgnorePrefixes: []string{"var/log"}})
	if d.Summary != (rd.Summary{BeforeEntries: 1, AfterEntries: 1, Modified: 1, BeforePayloadBytes: 2, AfterPayloadBytes: 3, NetGrowthBytes: 1}) || len(d.Changes) != 1 || d.Changes[0].Path != "var/logger" {
		t.Fatalf("ignore was not symmetric: %+v", d)
	}
}

func TestCompareWarningsSurviveIgnores(t *testing.T) {
	file := compareFile("ignored", 9)
	before := rd.Manifest{Entries: []rd.Entry{file}, Warnings: []rd.Warning{
		{Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "z.vendor"},
		{Path: ".", Code: rd.WarningUncomparedMetadata, Key: "root.vendor"},
		{Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "a.vendor"},
	}}
	after := rd.Manifest{Entries: []rd.Entry{file}, Warnings: []rd.Warning{
		{Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "z.vendor"},
		{Path: ".", Code: rd.WarningUncomparedMetadata, Key: "root.vendor"},
	}}
	wantWarnings := []rd.Warning{
		{Source: rd.SourceAfter, Path: ".", Code: rd.WarningUncomparedMetadata, Key: "root.vendor"},
		{Source: rd.SourceBefore, Path: ".", Code: rd.WarningUncomparedMetadata, Key: "root.vendor"},
		{Source: rd.SourceBefore, Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "a.vendor"},
		{Source: rd.SourceAfter, Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "z.vendor"},
		{Source: rd.SourceBefore, Path: "ignored", Code: rd.WarningUncomparedMetadata, Key: "z.vendor"},
	}
	for _, opts := range []rd.CompareOptions{{IgnorePaths: []string{"ignored"}}, {IgnorePrefixes: []string{"ignored"}}} {
		d := requireComparison(t, before, after, opts)
		compareCheck(t, d, rd.Diff{Changes: []rd.Change{}, PositiveGrowth: []rd.Growth{}, Warnings: wantWarnings})
	}
}

func TestCompareOrderStabilityAndInputIndependence(t *testing.T) {
	old, next := compareFile("modify", 2), compareFile("modify", 7)
	next.SHA256 = compareDigestB
	removed, added, same := compareFile("remove", 4), compareFile("add", 3), compareFile("same", 1)
	directory := rd.Entry{Path: "explicit", Kind: rd.KindDirectory, Mode: 0755}
	before := rd.Manifest{Entries: []rd.Entry{same, old, removed, directory}, Warnings: []rd.Warning{{Path: "modify", Code: rd.WarningUncomparedMetadata, Key: "z"}, {Path: "same", Code: rd.WarningUncomparedMetadata, Key: "a"}}}
	after := rd.Manifest{Entries: []rd.Entry{added, next, same, directory}, Warnings: []rd.Warning{{Path: "same", Code: rd.WarningUncomparedMetadata, Key: "a"}, {Path: "modify", Code: rd.WarningUncomparedMetadata, Key: "a"}}}
	opts := rd.CompareOptions{IgnorePaths: []string{"absent", "absent", "explicit"}, IgnorePrefixes: []string{"missing", "also-missing"}}
	want := rd.Diff{
		Summary: rd.Summary{BeforeEntries: 3, AfterEntries: 3, Added: 1, Removed: 1, Modified: 1, Unchanged: 1, BeforePayloadBytes: 7, AfterPayloadBytes: 11, NetGrowthBytes: 4},
		Changes: []rd.Change{
			{Path: "add", Kind: rd.ChangeAdded, After: &added, ChangedFields: []rd.Field{}},
			{Path: "modify", Kind: rd.ChangeModified, Before: &old, After: &next, ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256}},
			{Path: "remove", Kind: rd.ChangeRemoved, Before: &removed, ChangedFields: []rd.Field{}},
		},
		PositiveGrowth: []rd.Growth{{Path: "modify", Bytes: 5}, {Path: "add", Bytes: 3}},
		Warnings: []rd.Warning{
			{Source: rd.SourceAfter, Path: "modify", Code: rd.WarningUncomparedMetadata, Key: "a"},
			{Source: rd.SourceBefore, Path: "modify", Code: rd.WarningUncomparedMetadata, Key: "z"},
			{Source: rd.SourceAfter, Path: "same", Code: rd.WarningUncomparedMetadata, Key: "a"},
			{Source: rd.SourceBefore, Path: "same", Code: rd.WarningUncomparedMetadata, Key: "a"},
		},
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		rng.Shuffle(len(before.Entries), func(i, j int) { before.Entries[i], before.Entries[j] = before.Entries[j], before.Entries[i] })
		rng.Shuffle(len(after.Entries), func(i, j int) { after.Entries[i], after.Entries[j] = after.Entries[j], after.Entries[i] })
		rng.Shuffle(len(before.Warnings), func(i, j int) { before.Warnings[i], before.Warnings[j] = before.Warnings[j], before.Warnings[i] })
		rng.Shuffle(len(after.Warnings), func(i, j int) { after.Warnings[i], after.Warnings[j] = after.Warnings[j], after.Warnings[i] })
		rng.Shuffle(len(opts.IgnorePaths), func(i, j int) { opts.IgnorePaths[i], opts.IgnorePaths[j] = opts.IgnorePaths[j], opts.IgnorePaths[i] })
		rng.Shuffle(len(opts.IgnorePrefixes), func(i, j int) {
			opts.IgnorePrefixes[i], opts.IgnorePrefixes[j] = opts.IgnorePrefixes[j], opts.IgnorePrefixes[i]
		})
		beforeCopy := rd.Manifest{Entries: slices.Clone(before.Entries), Warnings: slices.Clone(before.Warnings)}
		afterCopy := rd.Manifest{Entries: slices.Clone(after.Entries), Warnings: slices.Clone(after.Warnings)}
		optsCopy := rd.CompareOptions{IgnorePaths: slices.Clone(opts.IgnorePaths), IgnorePrefixes: slices.Clone(opts.IgnorePrefixes)}
		d := requireComparison(t, before, after, opts)
		compareCheck(t, d, want)
		gotJSON, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Fatalf("unstable JSON: %s", gotJSON)
		}
		if !reflect.DeepEqual(before, beforeCopy) || !reflect.DeepEqual(after, afterCopy) || !reflect.DeepEqual(opts, optsCopy) {
			t.Fatal("Compare mutated caller input")
		}
		// Result sides and warnings must not alias the caller's backing slices.
		d.Changes[0].After.Mode = 0
		d.Changes[1].Before.Size = 999
		d.Changes[1].After.Size = 999
		d.Changes[2].Before.Mode = 0
		d.Warnings[0].Key = "changed"
		if !reflect.DeepEqual(before, beforeCopy) || !reflect.DeepEqual(after, afterCopy) {
			t.Fatal("returned records alias caller input")
		}
	}
}

func TestCompareRejectsInvalidCallerModelsAndOptions(t *testing.T) {
	file := compareFile("hidden", 0)
	badMode := file
	badMode.Mode = 010000
	badSize := file
	badSize.Size = -1
	badHash := file
	badHash.SHA256 = "invalid"
	badKind := file
	badKind.Kind = "socket"
	badPath := file
	badPath.Path = "a/../b"
	badDevice := file
	badDevice.DeviceMajor = 1
	badTarget := rd.Entry{Path: "hidden", Kind: rd.KindSymlink, LinkTarget: string([]byte{0xff})}
	badEffective := compareLink("alias", "hidden", file)
	badEffective.UID = 12
	for name, m := range map[string]rd.Manifest{
		"mode": {Entries: []rd.Entry{badMode}}, "negative size": {Entries: []rd.Entry{badSize}},
		"hash": {Entries: []rd.Entry{badHash}}, "kind": {Entries: []rd.Entry{badKind}},
		"path": {Entries: []rd.Entry{badPath}}, "device": {Entries: []rd.Entry{badDevice}},
		"target UTF-8": {Entries: []rd.Entry{badTarget}}, "duplicate": {Entries: []rd.Entry{file, file}},
		"non-directory ancestor":  {Entries: []rd.Entry{file, compareFile("hidden/child", 0)}},
		"missing hardlink target": {Entries: []rd.Entry{compareLink("hidden", "missing", file)}},
		"hardlink cycle":          {Entries: []rd.Entry{compareLink("hidden", "hidden", file)}},
		"hardlink endpoint":       {Entries: []rd.Entry{compareLink("alias", "hidden", file), {Path: "hidden", Kind: rd.KindDirectory}}},
		"hardlink effective data": {Entries: []rd.Entry{badEffective, file}},
		"warning source":          {Entries: []rd.Entry{file}, Warnings: []rd.Warning{{Source: rd.SourceBefore, Path: "hidden", Code: rd.WarningUncomparedMetadata, Key: "vendor"}}},
		"warning absent path":     {Warnings: []rd.Warning{{Path: "hidden", Code: rd.WarningUncomparedMetadata, Key: "vendor"}}},
		"warning invalid key":     {Entries: []rd.Entry{file}, Warnings: []rd.Warning{{Path: "hidden", Code: rd.WarningUncomparedMetadata, Key: string([]byte{0xff})}}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, beforeSide := range []bool{false, true} {
				before, after := rd.Manifest{}, m
				if beforeSide {
					before, after = after, before
				}
				d, err := rd.Compare(before, after, rd.CompareOptions{IgnorePrefixes: []string{"hidden", "alias", "a"}})
				if !errors.Is(err, rd.ErrInvalidInput) {
					t.Fatalf("invalid caller model hidden by ignores: %v", err)
				}
				var typed *rd.Error
				if !errors.As(err, &typed) || !reflect.DeepEqual(d, rd.Diff{}) {
					t.Fatalf("failure must be typed and return no partial result: %+v, %v", d, err)
				}
			}
		})
	}
	for _, path := range []string{"", ".", "/", "/absolute", "a//b", "./a", "a/./b", "a/../b", "a/", string([]byte{0xff})} {
		for _, prefix := range []bool{false, true} {
			opts := rd.CompareOptions{IgnorePaths: []string{path}}
			if prefix {
				opts = rd.CompareOptions{IgnorePrefixes: []string{path}}
			}
			d, err := rd.Compare(rd.Manifest{}, rd.Manifest{}, opts)
			if !errors.Is(err, rd.ErrInvalidInput) || !reflect.DeepEqual(d, rd.Diff{}) {
				t.Fatalf("ignore %q prefix=%t accepted or partial: %+v, %v", path, prefix, d, err)
			}
		}
	}
}

func TestCompareCheckedPayloadExtremes(t *testing.T) {
	maxFile := compareFile("max", math.MaxInt64)
	maximum := rd.Manifest{Entries: []rd.Entry{maxFile, compareLink("alias", "max", maxFile)}}
	for _, tc := range []struct {
		name                            string
		before, after                   rd.Manifest
		beforeBytes, afterBytes, growth int64
	}{
		{"max growth", rd.Manifest{}, maximum, 0, math.MaxInt64, math.MaxInt64},
		{"max shrinkage", maximum, rd.Manifest{}, math.MaxInt64, 0, -math.MaxInt64},
		{"max unchanged", maximum, maximum, math.MaxInt64, math.MaxInt64, 0},
		{"max totals small growth", rd.Manifest{Entries: []rd.Entry{compareFile("max", math.MaxInt64-1)}}, rd.Manifest{Entries: []rd.Entry{maxFile}}, math.MaxInt64 - 1, math.MaxInt64, 1},
		{"offset additions and removals", rd.Manifest{Entries: []rd.Entry{compareFile("old", math.MaxInt64)}}, rd.Manifest{Entries: []rd.Entry{maxFile}}, math.MaxInt64, math.MaxInt64, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := requireComparison(t, tc.before, tc.after, rd.CompareOptions{})
			if d.Summary.BeforePayloadBytes != tc.beforeBytes || d.Summary.AfterPayloadBytes != tc.afterBytes || d.Summary.NetGrowthBytes != tc.growth {
				t.Fatalf("arithmetic error: %+v", d.Summary)
			}
		})
	}
	overflow := rd.Manifest{Entries: []rd.Entry{maxFile, compareFile("extra", 1)}}
	for _, opts := range []rd.CompareOptions{{}, {IgnorePaths: []string{"extra"}}, {IgnorePrefixes: []string{"max", "extra"}}} {
		for _, beforeSide := range []bool{false, true} {
			before, after := rd.Manifest{}, overflow
			if beforeSide {
				before, after = after, before
			}
			d, err := rd.Compare(before, after, opts)
			if !errors.Is(err, rd.ErrInvalidInput) || !strings.Contains(err.Error(), "overflows int64") || !reflect.DeepEqual(d, rd.Diff{}) {
				t.Fatalf("overflow not rejected before filtering: %+v, %v", d, err)
			}
		}
	}
}
