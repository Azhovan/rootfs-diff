package rootfsdiff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	rd "github.com/Azhovan/rootfs-diff"
)

const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func regular(path string) rd.Entry {
	return rd.Entry{Path: path, Kind: rd.KindRegular, Mode: 0644, SHA256: emptyDigest}
}

func readModel[T any](t *testing.T, name string) T {
	t.Helper()
	data, err := os.ReadFile("testdata/models/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, rd.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	var detail *rd.Error
	if !errors.As(err, &detail) {
		t.Fatalf("expected typed validation context, got %T", err)
	}
}

func TestCanonicalPaths(t *testing.T) {
	for _, path := range []string{"etc/device.conf", `a\b`, "é/文件", "a/*?[x]", "line\n\t\x1b[31m"} {
		if err := rd.ValidatePath(path); err != nil {
			t.Errorf("valid literal %q: %v", path, err)
		}
	}
	for _, path := range []string{"", ".", "/", "/etc", "../etc", "a/../b", "a/./b", "./a", "a//b", "a/", string([]byte{0xff})} {
		t.Run(fmt.Sprintf("%q", path), func(t *testing.T) { assertInvalid(t, rd.ValidatePath(path)) })
	}
}

func TestEntryValidation(t *testing.T) {
	for _, e := range readModel[rd.Manifest](t, "valid-manifest.json").Entries {
		if err := e.Validate(); err != nil {
			t.Errorf("valid %s entry: %v", e.Kind, err)
		}
	}
	for name, mutate := range map[string]func(*rd.Entry){
		"kind":            func(e *rd.Entry) { e.Kind = "socket" },
		"mode":            func(e *rd.Entry) { e.Mode = 010000 },
		"negative size":   func(e *rd.Entry) { e.Size = -1 },
		"short digest":    func(e *rd.Entry) { e.SHA256 = "abcd" },
		"bad digest":      func(e *rd.Entry) { e.SHA256 = strings.Repeat("g", 64) },
		"upper digest":    func(e *rd.Entry) { e.SHA256 = strings.ToUpper(emptyDigest) },
		"nonfile size":    func(e *rd.Entry) { e.Kind, e.SHA256, e.Size = rd.KindDirectory, "", 1 },
		"nonfile hash":    func(e *rd.Entry) { e.Kind = rd.KindDirectory },
		"device fields":   func(e *rd.Entry) { e.DeviceMajor = 1 },
		"negative device": func(e *rd.Entry) { e.Kind, e.SHA256, e.DeviceMinor = rd.KindCharDevice, "", -1 },
		"unrelated link":  func(e *rd.Entry) { e.LinkTarget = "target" },
		"hardlink path":   func(e *rd.Entry) { e.Kind, e.LinkTarget = rd.KindHardlink, "../target" },
		"symlink UTF-8":   func(e *rd.Entry) { e.Kind, e.SHA256, e.LinkTarget = rd.KindSymlink, "", string([]byte{0xff}) },
	} {
		t.Run(name, func(t *testing.T) {
			e := regular("file")
			mutate(&e)
			assertInvalid(t, e.Validate())
		})
	}
	// The model has no scan cap; valid int64 extremes preserve tar metadata.
	e := regular("large")
	e.Size, e.UID, e.GID = math.MaxInt64, math.MinInt64, math.MaxInt64
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestValidationAndInputOrder(t *testing.T) {
	m := readModel[rd.Manifest](t, "valid-manifest.json")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(m.Entries)
	before, _ := json.Marshal(m)
	if err := m.Validate(); err != nil {
		t.Fatalf("caller entry order must be accepted: %v", err)
	}
	after, _ := json.Marshal(m)
	if !bytes.Equal(before, after) {
		t.Fatal("validation mutated input")
	}
	if err := (rd.Manifest{}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range readModel[[]struct {
		Name     string      `json:"name"`
		Manifest rd.Manifest `json:"manifest"`
	}](t, "invalid-manifests.json") {
		t.Run(fixture.Name, func(t *testing.T) { assertInvalid(t, fixture.Manifest.Validate()) })
	}
	for name, entries := range map[string][]rd.Entry{
		"nondirectory ancestor": {regular("a"), regular("a/b/c")},
		"symlink ancestor":      {{Path: "a", Kind: rd.KindSymlink, LinkTarget: "target"}, regular("a/b")},
		"hardlink self cycle":   {alias("a", "a")},
		"hardlink cycle":        {alias("a", "b"), alias("b", "a")},
		"nonregular endpoint":   {alias("a", "b"), {Path: "b", Kind: rd.KindDirectory}},
		"effective size":        {alias("a", "b"), withSize(regular("b"), 1)},
		"effective mode":        {alias("a", "b"), {Path: "b", Kind: rd.KindRegular, Mode: 0755, SHA256: emptyDigest}},
		"effective uid":         {alias("a", "b"), {Path: "b", Kind: rd.KindRegular, Mode: 0644, UID: 1, SHA256: emptyDigest}},
		"effective gid":         {alias("a", "b"), {Path: "b", Kind: rd.KindRegular, Mode: 0644, GID: 1, SHA256: emptyDigest}},
		"effective digest":      {alias("a", "b"), {Path: "b", Kind: rd.KindRegular, Mode: 0644, SHA256: strings.Repeat("a", 64)}},
	} {
		t.Run(name, func(t *testing.T) { assertInvalid(t, (rd.Manifest{Entries: entries}).Validate()) })
	}
	// A chain may point forward, uses no recursive call stack, and aliases do
	// not make an otherwise valid regular payload sum overflow.
	const count = 2000
	chain := make([]rd.Entry, 0, count+1)
	for i := range count {
		e := alias(fmt.Sprintf("link-%04d", i), fmt.Sprintf("link-%04d", i+1))
		e.Size = math.MaxInt64
		chain = append(chain, e)
	}
	terminal := regular(fmt.Sprintf("link-%04d", count))
	terminal.Size = math.MaxInt64
	chain = append(chain, terminal)
	if err := (rd.Manifest{Entries: chain}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func alias(path, target string) rd.Entry {
	e := regular(path)
	e.Kind, e.LinkTarget = rd.KindHardlink, target
	return e
}

func withSize(e rd.Entry, n int64) rd.Entry { e.Size = n; return e }

func TestChangeValidation(t *testing.T) {
	before, after := regular("file"), regular("file")
	after.Kind, after.Mode, after.UID, after.GID, after.Size, after.SHA256, after.LinkTarget = rd.KindHardlink, 06751, 1, 2, 3, strings.Repeat("a", 64), "elsewhere"
	c := rd.Change{Path: "file", Kind: rd.ChangeModified, Before: &before, After: &after,
		ChangedFields: []rd.Field{rd.FieldKind, rd.FieldMode, rd.FieldUID, rd.FieldGID, rd.FieldSize, rd.FieldSHA256, rd.FieldLinkTarget}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*rd.Change){
		"missing field":    func(c *rd.Change) { c.ChangedFields = c.ChangedFields[:1] },
		"field order":      func(c *rd.Change) { c.ChangedFields = []rd.Field{rd.FieldMode, rd.FieldKind} },
		"missing side":     func(c *rd.Change) { c.After = nil },
		"wrong path":       func(c *rd.Change) { c.Path = "other" },
		"bad kind":         func(c *rd.Change) { c.Kind = "same" },
		"bad added":        func(c *rd.Change) { c.Kind = rd.ChangeAdded },
		"bad removed":      func(c *rd.Change) { c.Kind = rd.ChangeRemoved },
		"no actual change": func(c *rd.Change) { c.After = c.Before; c.ChangedFields = nil },
	} {
		t.Run(name, func(t *testing.T) { copy := c; mutate(&copy); assertInvalid(t, copy.Validate()) })
	}
	deviceA := rd.Entry{Path: "dev/x", Kind: rd.KindCharDevice, DeviceMajor: 1}
	deviceB := rd.Entry{Path: "dev/x", Kind: rd.KindBlockDevice, DeviceMajor: 2, DeviceMinor: 3}
	if err := (rd.Change{Path: "dev/x", Kind: rd.ChangeModified, Before: &deviceA, After: &deviceB,
		ChangedFields: []rd.Field{rd.FieldKind, rd.FieldDeviceMajor, rd.FieldDeviceMinor}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDiffAndReportConsistency(t *testing.T) {
	valid := readModel[rd.Report](t, "report-v1.json")
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*rd.Report){
		"version":                func(r *rd.Report) { r.SchemaVersion++ },
		"negative count":         func(r *rd.Report) { r.Summary.Added = -1 },
		"wrong count":            func(r *rd.Report) { r.Summary.Added++ },
		"count overflow":         func(r *rd.Report) { r.Summary.Modified = math.MaxInt64 },
		"negative bytes":         func(r *rd.Report) { r.Summary.BeforePayloadBytes = -1 },
		"wrong growth":           func(r *rd.Report) { r.Summary.NetGrowthBytes++ },
		"unaccounted payload":    func(r *rd.Report) { r.Summary.BeforePayloadBytes++; r.Summary.NetGrowthBytes-- },
		"wrong change counts":    func(r *rd.Report) { r.Summary.Added = 0; r.Summary.AfterEntries-- },
		"changes unsorted":       func(r *rd.Report) { slices.Reverse(r.Changes) },
		"changes duplicate":      func(r *rd.Report) { r.Changes = append(r.Changes, r.Changes[0]) },
		"growth missing":         func(r *rd.Report) { r.PositiveGrowth = nil },
		"growth unsorted":        func(r *rd.Report) { slices.Reverse(r.PositiveGrowth) },
		"growth incorrect":       func(r *rd.Report) { r.PositiveGrowth[0].Bytes++ },
		"growth zero":            func(r *rd.Report) { r.PositiveGrowth[0].Bytes = 0 },
		"growth duplicate":       func(r *rd.Report) { r.PositiveGrowth[1] = r.PositiveGrowth[0] },
		"violations unsorted":    func(r *rd.Report) { slices.Reverse(r.Violations) },
		"violation wrong actual": func(r *rd.Report) { n := int64(2); r.Violations[0].Actual = &n },
		"missing exclusions":     func(r *rd.Report) { r.MetadataExclusions = nil },
		"exclusions unsorted":    func(r *rd.Report) { slices.Reverse(r.MetadataExclusions) },
		"warnings unsourced":     func(r *rd.Report) { r.Warnings[0].Source = "" },
		"warnings unsorted":      func(r *rd.Report) { slices.Reverse(r.Warnings) },
		"warnings duplicate":     func(r *rd.Report) { r.Warnings[1] = r.Warnings[0] },
	} {
		t.Run(name, func(t *testing.T) {
			r := readModel[rd.Report](t, "report-v1.json")
			mutate(&r)
			assertInvalid(t, r.Validate())
		})
	}
	// Both sides can approach int64 maximum without overflowing subtraction.
	d := rd.Diff{Summary: rd.Summary{BeforeEntries: 1, AfterEntries: 1, Unchanged: 1, BeforePayloadBytes: math.MaxInt64, AfterPayloadBytes: math.MaxInt64}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	assertInvalid(t, (rd.Summary{BeforePayloadBytes: 1, AfterPayloadBytes: 1}).Validate())
	d = rd.Diff{Summary: rd.Summary{BeforeEntries: 1, AfterEntries: 1, Modified: 1, BeforePayloadBytes: 1, AfterPayloadBytes: 1}, Changes: []rd.Change{{Path: "file", Kind: rd.ChangeModified, Before: ptr(regular("file")), After: ptr(withMode(regular("file"), 0755)), ChangedFields: []rd.Field{rd.FieldMode}}}}
	assertInvalid(t, d.Validate()) // no unchanged entry can own the unexplained byte
}

func ptr[T any](v T) *T                      { return &v }
func withMode(e rd.Entry, n uint32) rd.Entry { e.Mode = n; return e }

func TestPolicyWarningViolationValidation(t *testing.T) {
	if err := readModel[rd.Policy](t, "policy-v1.json").Validate(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []rd.Policy{{SchemaVersion: 1}, {SchemaVersion: 1, RequiredPaths: []string{"z", "a", "a"}, MaxTotalGrowthBytes: ptr(int64(0))}} {
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []rd.Policy{{}, {SchemaVersion: 2}, {SchemaVersion: 1, RequiredPaths: []string{"../bad"}}, {SchemaVersion: 1, MaxTotalGrowthBytes: ptr(int64(-1))}} {
		assertInvalid(t, p.Validate())
	}
	w := rd.Warning{Path: "file", Code: rd.WarningUncomparedMetadata, Key: "VENDOR.note"}
	for _, bad := range []rd.Warning{{Path: "file", Code: "unknown", Key: "key"}, {Path: "file", Code: rd.WarningUncomparedMetadata}, {Path: "file", Code: rd.WarningUncomparedMetadata, Key: string([]byte{255})}, {Path: "file", Code: rd.WarningUncomparedMetadata, Key: "key", Source: "unknown"}} {
		assertInvalid(t, bad.Validate())
	}
	if err := (rd.Manifest{Entries: []rd.Entry{regular("file")}, Warnings: []rd.Warning{w}}).Validate(); err != nil {
		t.Fatal(err)
	}
	assertInvalid(t, (rd.Manifest{Warnings: []rd.Warning{w}}).Validate())
	assertInvalid(t, (rd.Manifest{Entries: []rd.Entry{regular("file")}, Warnings: []rd.Warning{w, w}}).Validate())
	w.Source = rd.SourceAfter
	assertInvalid(t, (rd.Manifest{Entries: []rd.Entry{regular("file")}, Warnings: []rd.Warning{w}}).Validate())
	for _, v := range []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "file"}, {Rule: rd.RuleMaxTotalGrowthBytes, Limit: ptr(int64(0)), Actual: ptr(int64(1))}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []rd.Violation{{}, {Rule: rd.RuleRequiredPath}, {Rule: rd.RuleRequiredPath, Path: "file", Limit: ptr(int64(0))}, {Rule: rd.RuleMaxTotalGrowthBytes}, {Rule: rd.RuleMaxTotalGrowthBytes, Limit: ptr(int64(-1)), Actual: ptr(int64(1))}, {Rule: rd.RuleMaxTotalGrowthBytes, Limit: ptr(int64(1)), Actual: ptr(int64(1))}} {
		assertInvalid(t, v.Validate())
	}
}

func TestIgnoredRootMetadataWarnings(t *testing.T) {
	m := readModel[rd.Manifest](t, "root-warning-manifest.json")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 0 || len(m.Warnings) != 1 || m.Warnings[0].Path != "." {
		t.Fatal("root warning must not require or synthesize a root entry")
	}
	encoded, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/models/root-warning-manifest.json")
	if err != nil || !bytes.Equal(append(encoded, '\n'), want) {
		t.Fatalf("root warning manifest round trip differs: %s, %v", encoded, err)
	}
	// The exception is exact and confined to warnings. Entry and option paths
	// still use canonical nonroot rules, including hardlink/policy paths.
	assertInvalid(t, (rd.Entry{Path: ".", Kind: rd.KindDirectory}).Validate())
	assertInvalid(t, alias("alias", ".").Validate())
	assertInvalid(t, (rd.CompareOptions{IgnorePaths: []string{"."}}).Validate())
	assertInvalid(t, (rd.CompareOptions{IgnorePrefixes: []string{"."}}).Validate())
	assertInvalid(t, (rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"."}}).Validate())
	for _, path := range []string{"", "./", "/", "./file"} {
		w := m.Warnings[0]
		w.Path = path
		assertInvalid(t, w.Validate())
	}
	for _, mutate := range []func(*rd.Warning){
		func(w *rd.Warning) { w.Source = rd.SourceBefore },
		func(w *rd.Warning) { w.Key = string([]byte{0xff}) },
	} {
		w := m.Warnings[0]
		mutate(&w)
		assertInvalid(t, (rd.Manifest{Warnings: []rd.Warning{w}}).Validate())
	}
	assertInvalid(t, (rd.Manifest{Warnings: []rd.Warning{m.Warnings[0], m.Warnings[0]}}).Validate())
	r := readModel[rd.Report](t, "root-warning-report-v1.json")
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(r.Changes) != 0 || r.Summary != (rd.Summary{}) || len(r.Warnings) != 2 || r.Warnings[0].Source != rd.SourceAfter || r.Warnings[1].Source != rd.SourceBefore {
		t.Fatal("root warnings must keep both sources without adding entries, changes or payload")
	}
	r.Warnings[0].Source = ""
	assertInvalid(t, r.Validate())
}

func TestModelJSONRoundTrip(t *testing.T) {
	for _, name := range []string{"report-v1.json", "empty-report-v1.json", "root-warning-report-v1.json"} {
		t.Run(name, func(t *testing.T) {
			r := readModel[rd.Report](t, name)
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			want, _ := os.ReadFile("testdata/models/" + name)
			if !bytes.Equal(append(got, '\n'), want) {
				t.Fatalf("schema field order/values changed:\n%s", got)
			}
			var roundTrip rd.Report
			if err := json.Unmarshal(got, &roundTrip); err != nil || !reflect.DeepEqual(r, roundTrip) {
				t.Fatalf("round trip differs: %v", err)
			}
		})
	}
	// Explicit side nulls and empty arrays survive even nil Go slices.
	for _, tc := range []struct {
		value any
		want  string
	}{
		{rd.Manifest{}, `{"entries":[],"warnings":[]}`},
		{rd.CompareOptions{}, `{"ignore_paths":[],"ignore_prefixes":[]}`},
		{rd.Policy{SchemaVersion: 1}, `{"schema_version":1,"required_paths":[]}`},
		{rd.Change{Path: "x", Kind: rd.ChangeAdded}, `{"path":"x","kind":"added","before":null,"after":null,"changed_fields":[]}`},
	} {
		got, err := json.Marshal(tc.value)
		if err != nil || string(got) != tc.want {
			t.Errorf("%T = %s, %v; want %s", tc.value, got, err, tc.want)
		}
	}
	for _, data := range []string{`{"path":"file","kind":"regular","size":9223372036854775808}`, `{"path":"file","kind":"regular","mode":-1}`, `{"path":"file","kind":"regular","size":1.5}`} {
		var e rd.Entry
		if err := json.Unmarshal([]byte(data), &e); err == nil {
			t.Errorf("accepted unrepresentable numeric model: %s", data)
		}
	}
	r := readModel[rd.Report](t, "report-v1.json")
	if r.Changes[0].Before.Mode != 0640 || r.Changes[0].After.Mode != 06750 || !slices.Equal(r.Changes[0].ChangedFields, []rd.Field{rd.FieldMode, rd.FieldSize, rd.FieldSHA256}) || r.Summary.NetGrowthBytes != 1 {
		t.Fatal("independently authored report changed mode bits, changed fields or growth")
	}
	list := rd.MetadataExclusions()
	list[0] = "mutated"
	if rd.MetadataExclusions()[0] != "acls" {
		t.Fatal("exclusions exposes mutable shared state")
	}
}

func TestOptions(t *testing.T) {
	o := rd.ScanOptions{}
	effective, err := o.WithDefaults()
	if err != nil || effective != (rd.ScanOptions{MaxEntries: 100000, MaxArchiveBytes: 8589934592, MaxFileBytes: 2147483648, MaxPathBytes: 4096}) || o != (rd.ScanOptions{}) {
		t.Fatalf("defaults = %+v, err %v; original %+v", effective, err, o)
	}
	for _, o := range []rd.ScanOptions{{MaxEntries: -1}, {MaxArchiveBytes: -1}, {MaxFileBytes: -1}, {MaxPathBytes: -1}} {
		assertInvalid(t, o.Validate())
	}
	// Cross-budget relationships need not be restricted: the first cap wins.
	for _, o := range []rd.ScanOptions{{MaxEntries: 1, MaxArchiveBytes: 1, MaxFileBytes: math.MaxInt64, MaxPathBytes: math.MaxInt64}, {}} {
		if err := o.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []rd.CompareOptions{{}, {IgnorePaths: []string{"a", "a"}, IgnorePrefixes: []string{"z", "a"}}} {
		if err := o.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	assertInvalid(t, (rd.CompareOptions{IgnorePaths: []string{"./a"}}).Validate())
	assertInvalid(t, (rd.CompareOptions{IgnorePrefixes: []string{"a/"}}).Validate())
}

func TestErrorIdentity(t *testing.T) {
	for _, kind := range []rd.ErrorKind{rd.ErrorInvalidInput, rd.ErrorUnsupportedInput, ""} {
		err := &rd.Error{Op: "scan", Path: "file\n", Kind: kind, Err: io.ErrUnexpectedEOF}
		wrapped := fmt.Errorf("outer: %w", err)
		if !errors.Is(wrapped, io.ErrUnexpectedEOF) || errors.Is(wrapped, rd.ErrInvalidInput) != (kind == rd.ErrorInvalidInput) || errors.Is(wrapped, rd.ErrUnsupportedInput) != (kind == rd.ErrorUnsupportedInput) {
			t.Fatalf("error identity lost: %v", wrapped)
		}
		var detail *rd.Error
		if !errors.As(wrapped, &detail) || detail.Path != "file\n" || strings.Contains(err.Error(), "file\n") {
			t.Fatalf("typed context or escaped diagnostic lost: %v", wrapped)
		}
	}
	limit := &rd.LimitError{Limit: "archive_bytes", Maximum: 10, Actual: 11, Err: io.ErrUnexpectedEOF}
	err := fmt.Errorf("scan: %w", limit)
	var detail *rd.LimitError
	if !errors.Is(err, rd.ErrLimitExceeded) || !errors.Is(err, io.ErrUnexpectedEOF) || !errors.As(err, &detail) || detail.Maximum != 10 || detail.Actual != 11 {
		t.Fatalf("limit identity lost: %v", err)
	}
}
