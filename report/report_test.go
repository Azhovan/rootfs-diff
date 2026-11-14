package report_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	rd "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/report"
)

func fixture(t *testing.T, name string) rd.Report {
	t.Helper()
	b, err := os.ReadFile("../testdata/models/" + name + "-report-v1.json")
	if name == "complete" {
		b, err = os.ReadFile("../testdata/models/report-v1.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	var r rd.Report
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	return r
}

func allFields() rd.Report {
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	return rd.Report{
		SchemaVersion: 1,
		Summary: rd.Summary{BeforeEntries: 6, AfterEntries: 6, Added: 1, Removed: 1, Modified: 5,
			BeforePayloadBytes: 12, AfterPayloadBytes: 6, NetGrowthBytes: -6},
		Changes: []rd.Change{
			{Path: "block", Kind: rd.ChangeModified,
				Before:        &rd.Entry{Path: "block", Kind: rd.KindCharDevice, Mode: 0600, UID: -2, GID: 4, DeviceMajor: 1, DeviceMinor: 3},
				After:         &rd.Entry{Path: "block", Kind: rd.KindBlockDevice, Mode: 01640, UID: 3, GID: 5, DeviceMajor: 8, DeviceMinor: 9},
				ChangedFields: []rd.Field{rd.FieldKind, rd.FieldMode, rd.FieldUID, rd.FieldGID, rd.FieldDeviceMajor, rd.FieldDeviceMinor}},
			{Path: "dir", Kind: rd.ChangeAdded, After: &rd.Entry{Path: "dir", Kind: rd.KindDirectory, Mode: 01777, UID: 10, GID: 20}},
			{Path: "fifo", Kind: rd.ChangeRemoved, Before: &rd.Entry{Path: "fifo", Kind: rd.KindFIFO, Mode: 0660, UID: 30, GID: 40}},
			{Path: "hard", Kind: rd.ChangeModified,
				Before:        &rd.Entry{Path: "hard", Kind: rd.KindHardlink, Mode: 0644, UID: 1, GID: 2, Size: 5, SHA256: a, LinkTarget: "old"},
				After:         &rd.Entry{Path: "hard", Kind: rd.KindHardlink, Mode: 06750, UID: -1, GID: -2, Size: 8, SHA256: b, LinkTarget: "new"},
				ChangedFields: []rd.Field{rd.FieldMode, rd.FieldUID, rd.FieldGID, rd.FieldSize, rd.FieldSHA256, rd.FieldLinkTarget}},
			{Path: "reg", Kind: rd.ChangeModified,
				Before:        &rd.Entry{Path: "reg", Kind: rd.KindRegular, Mode: 0644, Size: 10, SHA256: a},
				After:         &rd.Entry{Path: "reg", Kind: rd.KindRegular, Mode: 0644, Size: 6, SHA256: b},
				ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256}},
			{Path: "sym", Kind: rd.ChangeModified,
				Before:        &rd.Entry{Path: "sym", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "old"},
				After:         &rd.Entry{Path: "sym", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "new"},
				ChangedFields: []rd.Field{rd.FieldLinkTarget}},
			{Path: "type", Kind: rd.ChangeModified,
				Before:        &rd.Entry{Path: "type", Kind: rd.KindRegular, Mode: 0644, Size: 2, SHA256: a},
				After:         &rd.Entry{Path: "type", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../reg"},
				ChangedFields: []rd.Field{rd.FieldKind, rd.FieldMode, rd.FieldSize, rd.FieldSHA256, rd.FieldLinkTarget}},
		},
		MetadataExclusions: rd.MetadataExclusions(),
	}
}

func scenarios(t *testing.T) map[string]rd.Report {
	t.Helper()
	empty := fixture(t, "empty")
	noChange := fixture(t, "empty")
	noChange.Summary = rd.Summary{BeforeEntries: 1, AfterEntries: 1, Unchanged: 1, BeforePayloadBytes: 2, AfterPayloadBytes: 2}
	shrink := fixture(t, "complete")
	c := &shrink.Changes[0]
	c.Before, c.After = c.After, c.Before
	shrink.Summary.BeforePayloadBytes = 10
	shrink.Summary.AfterPayloadBytes = 7
	shrink.Summary.NetGrowthBytes = -3
	shrink.PositiveGrowth = []rd.Growth{{Path: "etc/new", Bytes: 2}}
	shrink.Violations = shrink.Violations[1:]
	return map[string]rd.Report{"empty": empty, "no-change": noChange, "complete": fixture(t, "complete"), "shrink": shrink, "all-fields": allFields()}
}

func render(t *testing.T, r rd.Report, f report.Format) string {
	t.Helper()
	var out bytes.Buffer
	if err := report.Write(&out, r, f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestHumanGoldens(t *testing.T) {
	for name, r := range scenarios(t) {
		for _, format := range []report.Format{report.Text, report.Markdown} {
			t.Run(name+"/"+string(format), func(t *testing.T) {
				ext := ".text"
				if format == report.Markdown {
					ext = ".md"
				}
				want, err := os.ReadFile("testdata/" + name + ext)
				if err != nil {
					t.Fatal(err)
				}
				if got := render(t, r, format); got != string(want) {
					t.Fatalf("golden mismatch\ngot:\n%s\nwant:\n%s", got, want)
				}
			})
		}
	}
}

func TestJSONSchemaValuesAndRecords(t *testing.T) {
	for name, r := range scenarios(t) {
		t.Run(name, func(t *testing.T) {
			got := render(t, r, report.JSON)
			var decoded rd.Report
			dec := json.NewDecoder(strings.NewReader(got))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&decoded); err != nil {
				t.Fatal(err)
			}
			if err := decoded.Validate(); err != nil {
				t.Fatal(err)
			}
			if decoded.SchemaVersion != 1 || decoded.Summary != r.Summary || !slices.Equal(decoded.PositiveGrowth, r.PositiveGrowth) ||
				!reflect.DeepEqual(decoded.Changes, nonnilChanges(r.Changes)) || !reflect.DeepEqual(decoded.Violations, nonnil(r.Violations)) ||
				!slices.Equal(decoded.Warnings, r.Warnings) || !slices.Equal(decoded.MetadataExclusions, rd.MetadataExclusions()) {
				t.Fatalf("JSON lost fields: %+v", decoded)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal([]byte(got), &object); err != nil {
				t.Fatal(err)
			}
			if len(object) != 7 {
				t.Fatalf("top-level fields: %v", object)
			}
			for _, key := range []string{"changes", "positive_growth", "violations", "warnings", "metadata_exclusions"} {
				if len(object[key]) == 0 || object[key][0] != '[' {
					t.Fatalf("%s must be an array: %s", key, object[key])
				}
			}
			var changes []map[string]json.RawMessage
			if err := json.Unmarshal(object["changes"], &changes); err != nil {
				t.Fatal(err)
			}
			for i, change := range changes {
				if len(change) != 5 || len(change["changed_fields"]) == 0 || change["changed_fields"][0] != '[' {
					t.Fatalf("incomplete change %d: %v", i, change)
				}
				for _, side := range []string{"before", "after"} {
					if string(change[side]) == "null" {
						continue
					}
					var entry map[string]json.RawMessage
					if err := json.Unmarshal(change[side], &entry); err != nil || len(entry) != 10 {
						t.Fatalf("incomplete %s entry: %s (%v)", side, change[side], err)
					}
				}
			}
			if name == "complete" {
				if decoded.Summary.NetGrowthBytes != 1 || decoded.Changes[0].After.Mode != 3560 || decoded.PositiveGrowth[1].Bytes != 2 ||
					decoded.Changes[1].Before != nil || decoded.Changes[2].After != nil || *decoded.Violations[0].Actual != 1 ||
					decoded.Violations[1].Limit != nil || decoded.Warnings[0].Source != rd.SourceAfter || decoded.Warnings[1].Source != rd.SourceBefore {
					t.Fatal("independent fixture value mismatch")
				}
				if string(changes[1]["before"]) != "null" || string(changes[2]["after"]) != "null" || string(changes[1]["changed_fields"]) != "[]" {
					t.Fatal("absent side/null/empty array contract violated")
				}
			}
		})
	}
	// The pre-existing foundation JSON expectations were authored independently.
	for _, name := range []string{"empty", "complete", "root-warning"} {
		r := fixture(t, name)
		path := "../testdata/models/" + name + "-report-v1.json"
		if name == "complete" {
			path = "../testdata/models/report-v1.json"
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := render(t, r, report.JSON); got != string(want) {
			t.Fatalf("JSON golden %s mismatch", name)
		}
	}
}

func nonnil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func nonnilChanges(in []rd.Change) []rd.Change {
	out := append([]rd.Change{}, in...)
	for i := range out {
		out[i].ChangedFields = nonnil(out[i].ChangedFields)
	}
	return out
}

func TestStableBytesAndImmutability(t *testing.T) {
	for name, r := range scenarios(t) {
		for _, f := range []report.Format{report.Text, report.JSON, report.Markdown} {
			t.Run(name+"/"+string(f), func(t *testing.T) {
				// JSON cloning is solely for before/after comparison, not expected rendering.
				before, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				var clone rd.Report
				if err := json.Unmarshal(before, &clone); err != nil {
					t.Fatal(err)
				}
				want := render(t, r, f)
				for range 3 {
					if got := render(t, clone, f); got != want {
						t.Fatal("equivalent models produced different bytes")
					}
				}
				after, err := json.Marshal(r)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("input mutated: %v", err)
				}
				// Marshal normalizes nil arrays, so check nil shape separately.
				if name == "all-fields" && (r.PositiveGrowth != nil || r.Violations != nil || r.Warnings != nil || r.Changes[1].ChangedFields != nil) {
					t.Fatal("input nil slices were mutated")
				}
			})
		}
	}
}

func TestGrowthTopTenAndEveryChange(t *testing.T) {
	r := rd.Report{SchemaVersion: 1, MetadataExclusions: rd.MetadataExclusions()}
	for i := range 12 {
		p := fmt.Sprintf("file-%02d", i)
		r.Changes = append(r.Changes, rd.Change{Path: p, Kind: rd.ChangeAdded,
			After: &rd.Entry{Path: p, Kind: rd.KindRegular, Size: 3, SHA256: strings.Repeat("a", 64)}})
		r.PositiveGrowth = append(r.PositiveGrowth, rd.Growth{Path: p, Bytes: 3})
	}
	r.Summary = rd.Summary{AfterEntries: 12, Added: 12, AfterPayloadBytes: 36, NetGrowthBytes: 36}
	for _, f := range []report.Format{report.Text, report.Markdown} {
		out := render(t, r, f)
		growth := strings.Split(strings.Split(out, "Positive growth (showing 10 of 12)")[1], "Changes (12)")[0]
		if strings.Count(growth, "+3 bytes") != 10 || strings.Count(out, "added ") != 13 || strings.Count(out, "After: kind=regular") != 12 {
			// One "added " occurs in the summary.
			t.Fatalf("wrong growth/change counts for %s: %s", f, out)
		}
	}
	var decoded rd.Report
	if err := json.Unmarshal([]byte(render(t, r, report.JSON)), &decoded); err != nil || len(decoded.PositiveGrowth) != 12 || len(decoded.Changes) != 12 {
		t.Fatalf("JSON truncated: %+v (%v)", decoded, err)
	}
}

func maliciousReport(s string) rd.Report {
	path := "path-" + s
	return rd.Report{SchemaVersion: 1,
		Summary: rd.Summary{AfterEntries: 1, Added: 1},
		Changes: []rd.Change{{Path: path, Kind: rd.ChangeAdded,
			After: &rd.Entry{Path: path, Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "target-" + s}}},
		Violations: []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "missing-" + s}},
		Warnings: []rd.Warning{{Source: rd.SourceAfter, Path: ".", Code: rd.WarningUncomparedMetadata, Key: "key-" + s},
			{Source: rd.SourceBefore, Path: path, Code: rd.WarningUncomparedMetadata, Key: "key-" + s}},
		MetadataExclusions: rd.MetadataExclusions()}
}

func TestUntrustedStrings(t *testing.T) {
	cases := []string{"|`<>&\n\t\r\x1b[31m雪", "\x00\x07\x08\x7f\u0085\u009b31m\u202e\u2066\u2028\u2029\U000e0001", "**bold** _em_ ~~strike~~ [link](javascript:evil) ![image](url) # heading \\ &lt;tag&gt; <script>x<" + "/script>", strings.Repeat("long-雪", 2000)}
	for i, s := range cases {
		r := maliciousReport(s)
		for _, f := range []report.Format{report.Text, report.Markdown, report.JSON} {
			t.Run(fmt.Sprintf("%d/%s", i, f), func(t *testing.T) {
				out := render(t, r, f)
				for _, ch := range out {
					if ch != '\n' && (unicode.IsControl(ch) || unicode.In(ch, unicode.Cf)) {
						t.Fatalf("unsafe control U+%04X", ch)
					}
				}
				if f == report.Markdown {
					if strings.ContainsAny(out, "|`") {
						t.Fatal("untrusted Markdown punctuation was not encoded")
					}
					if strings.Contains(out, "<script>") || strings.Contains(out, "javascript:evil") || strings.Contains(out, "&lt;tag&gt;") {
						t.Fatal("untrusted markup escaped incorrectly")
					}
					for _, token := range []string{"path", "target", "missing", "key"} {
						if !strings.Contains(out, "<code>&#34;"+token+"&#45;") {
							t.Fatalf("missing escaped %s", token)
						}
					}
					var decoded []string
					for _, match := range regexp.MustCompile(`<code>([^<]*)</code>`).FindAllStringSubmatch(out, -1) {
						decoded = append(decoded, html.UnescapeString(match[1]))
					}
					for _, token := range []string{"path-", "target-", "missing-", "key-"} {
						if !slices.Contains(decoded, strconv.Quote(token+s)) {
							t.Fatalf("Markdown lost literal %s value", token)
						}
					}
				} else if f == report.Text && i == 0 {
					for _, token := range []string{`"path-|` + "`" + `<>&\n\t\r\x1b[31m雪"`, `link_target="target-|` + "`" + `<>&\n\t\r\x1b[31m雪"`, `"missing-|` + "`" + `<>&\n\t\r\x1b[31m雪"`, `key "key-|` + "`" + `<>&\n\t\r\x1b[31m雪"`} {
						if !strings.Contains(out, token) {
							t.Fatalf("missing exact escaped token %q", token)
						}
					}
				}
				if f == report.JSON {
					var decoded rd.Report
					if err := json.Unmarshal([]byte(out), &decoded); err != nil || decoded.Changes[0].Path != "path-"+s || decoded.Changes[0].After.LinkTarget != "target-"+s || decoded.Warnings[0].Key != "key-"+s || decoded.Violations[0].Path != "missing-"+s {
						t.Fatalf("JSON did not preserve strings: %v", err)
					}
				}
				if i == 3 && strings.Count(out, "long-雪") == 0 && f != report.Markdown {
					t.Fatal("long strings lost")
				}
			})
		}
	}
}

type failingWriter struct {
	data []byte
	n    int
	err  error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	n := min(w.n, len(p))
	w.data = append(w.data, p[:n]...)
	return n, w.err
}

func TestWriterFailures(t *testing.T) {
	sentinel := errors.New("sink failed")
	for _, f := range []report.Format{report.Text, report.JSON, report.Markdown} {
		for _, tc := range []struct {
			n       int
			err     error
			wantErr error
		}{{0, sentinel, sentinel}, {13, sentinel, sentinel}, {0, nil, io.ErrShortWrite}, {13, nil, io.ErrShortWrite}} {
			w := &failingWriter{n: tc.n, err: tc.err}
			if err := report.Write(w, fixture(t, "complete"), f); !errors.Is(err, tc.wantErr) || len(w.data) != tc.n {
				t.Fatalf("%s: error %v, bytes %d; want %v/%d", f, err, len(w.data), tc.wantErr, tc.n)
			}
		}
	}
}

func TestRejectedBeforeWrite(t *testing.T) {
	for _, f := range []report.Format{report.Text, report.JSON, report.Markdown} {
		for _, mutate := range []func(*rd.Report){
			func(r *rd.Report) { r.SchemaVersion = 2 },
			func(r *rd.Report) { r.MetadataExclusions = nil },
			func(r *rd.Report) { r.Changes[0].ChangedFields = []rd.Field{rd.FieldMode} },
			func(r *rd.Report) { r.Changes[1].Before = r.Changes[1].After },
			func(r *rd.Report) { slices.Reverse(r.Changes) },
			func(r *rd.Report) { slices.Reverse(r.Warnings) },
			func(r *rd.Report) { slices.Reverse(r.Violations) },
			func(r *rd.Report) { r.PositiveGrowth = nil },
			func(r *rd.Report) { r.Summary.AfterPayloadBytes++ },
			func(r *rd.Report) { r.Changes[1].After.LinkTarget = "invalid for regular" },
		} {
			r := fixture(t, "complete")
			mutate(&r)
			var out bytes.Buffer
			if err := report.Write(&out, r, f); !errors.Is(err, rd.ErrInvalidInput) || out.Len() != 0 {
				t.Fatalf("%s invalid report: %v, %d bytes", f, err, out.Len())
			}
		}
		if err := report.Write(nil, fixture(t, "empty"), f); !errors.Is(err, rd.ErrInvalidInput) {
			t.Fatalf("nil writer: %v", err)
		}
	}
	for _, f := range []report.Format{"", "yaml", "\x1b[31m"} {
		var out bytes.Buffer
		err := report.Write(&out, fixture(t, "empty"), f)
		if !errors.Is(err, rd.ErrUnsupportedInput) || out.Len() != 0 || strings.ContainsRune(err.Error(), '\x1b') {
			t.Fatalf("unsupported format: %v, bytes %d", err, out.Len())
		}
	}
}
