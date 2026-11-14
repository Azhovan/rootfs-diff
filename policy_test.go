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
	"strings"
	"testing"

	rd "github.com/Azhovan/rootfs-diff"
)

const policyInputLimit = 1 << 20
const policyDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func policyInt(n int64) *int64 { return &n }

func policyFile(path string, size int64) rd.Entry {
	return rd.Entry{Path: path, Kind: rd.KindRegular, Mode: 0644, Size: size, SHA256: policyDigest}
}

func policyParse(t *testing.T, input string) rd.Policy {
	t.Helper()
	p, err := rd.ParsePolicy(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("successful ParsePolicy returned invalid policy: %v", err)
	}
	return p
}

func policyCompare(t *testing.T, before, after rd.Manifest, opts rd.CompareOptions) rd.Diff {
	t.Helper()
	d, err := rd.Compare(before, after, opts)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func policyEvaluate(t *testing.T, after rd.Manifest, diff rd.Diff, policy rd.Policy) []rd.Violation {
	t.Helper()
	got, err := rd.Evaluate(after, diff, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range got {
		if err := violation.Validate(); err != nil {
			t.Fatalf("Evaluate returned invalid violation: %v", err)
		}
	}
	return got
}

func TestParsePolicyValid(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		paths       []string
		limit       *int64
	}{
		{"disabled growth", `{"schema_version":1,"required_paths":[]}`, []string{}, nil},
		{"explicit zero", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":0}`, []string{}, policyInt(0)},
		{"maximum integer", `{"required_paths":["etc/init.d/S40network","usr/bin/agent"],"max_total_growth_bytes":9223372036854775807,"schema_version":1}`, []string{"etc/init.d/S40network", "usr/bin/agent"}, policyInt(math.MaxInt64)},
		{"literal path syntax", `{"schema_version":1,"required_paths":["a\\b","a*?[]","line\nbreak","caf\u00e9","\ud83d\ude00","\ufffd"]}`, []string{"a\\b", "a*?[]", "line\nbreak", "café", "😀", "�"}, nil},
		{"escaped backslash stays literal", `{"schema_version":1,"required_paths":["\\ud800","slash\\u1234"]}`, []string{"\\ud800", "slash\\u1234"}, nil},
		{"duplicate paths", `{"schema_version":1,"required_paths":["z","a","z"]}`, []string{"z", "a", "z"}, nil},
		{"surrounding whitespace", " \n\t{\"schema_version\":1,\"required_paths\":[]}\r\n", []string{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := policyParse(t, tc.input)
			if p.SchemaVersion != 1 || !reflect.DeepEqual(p.RequiredPaths, tc.paths) || !reflect.DeepEqual(p.MaxTotalGrowthBytes, tc.limit) {
				t.Fatalf("parsed policy = %#v, want version 1, paths %q, limit %v", p, tc.paths, tc.limit)
			}
		})
	}
}

func TestParsePolicySample(t *testing.T) {
	input, err := os.Open("testdata/policy/policy-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	got, err := rd.ParsePolicy(input)
	if err != nil {
		t.Fatal(err)
	}
	want := rd.Policy{
		SchemaVersion:       1,
		RequiredPaths:       []string{"etc/init.d/S40network", "usr/bin/device-agent"},
		MaxTotalGrowthBytes: policyInt(1048576),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sample policy = %#v, want %#v", got, want)
	}
}

func TestEvaluateEmptyRules(t *testing.T) {
	for _, paths := range [][]string{nil, {}} {
		got := policyEvaluate(t, rd.Manifest{}, rd.Diff{}, rd.Policy{SchemaVersion: 1, RequiredPaths: paths})
		if len(got) != 0 {
			t.Fatalf("empty rules produced violations: %#v", got)
		}
	}
}

func TestParsePolicyRejectsInvalidJSON(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"empty", ""},
		{"whitespace", " \t\n"},
		{"malformed", `{"schema_version":1,"required_paths":[}`},
		{"truncated", `{"schema_version":1,"required_paths":[]`},
		{"trailing comma", `{"schema_version":1,"required_paths":[],}`},
		{"comment", `{"schema_version":1,"required_paths":[]} // policy`},
		{"top level array", `[]`},
		{"top level string", `"policy"`},
		{"top level number", `1`},
		{"top level bool", `true`},
		{"top level null", `null`},
		{"missing version", `{"required_paths":[]}`},
		{"missing required paths", `{"schema_version":1}`},
		{"unknown field", `{"schema_version":1,"required_paths":[],"security":true}`},
		{"case variant field", `{"Schema_Version":1,"required_paths":[]}`},
		{"duplicate version", `{"schema_version":1,"required_paths":[],"schema_version":1}`},
		{"duplicate paths field", `{"schema_version":1,"required_paths":[],"required_paths":[]}`},
		{"duplicate limit", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":0,"max_total_growth_bytes":1}`},
		{"escaped duplicate key", `{"schema_version":1,"required_paths":[],"\u0073chema_version":1}`},
		{"nested duplicate object", `{"schema_version":1,"required_paths":[{"a":1,"a":2}]}`},
		{"nested unknown object", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":{"a":1}}`},
		{"version zero", `{"schema_version":0,"required_paths":[]}`},
		{"unknown version", `{"schema_version":2,"required_paths":[]}`},
		{"negative version", `{"schema_version":-1,"required_paths":[]}`},
		{"version overflow", `{"schema_version":9223372036854775808,"required_paths":[]}`},
		{"version string", `{"schema_version":"1","required_paths":[]}`},
		{"version fraction", `{"schema_version":1.0,"required_paths":[]}`},
		{"version exponent", `{"schema_version":1e0,"required_paths":[]}`},
		{"version null", `{"schema_version":null,"required_paths":[]}`},
		{"version bool", `{"schema_version":true,"required_paths":[]}`},
		{"paths null", `{"schema_version":1,"required_paths":null}`},
		{"paths string", `{"schema_version":1,"required_paths":"etc/agent"}`},
		{"paths object", `{"schema_version":1,"required_paths":{}}`},
		{"path null", `{"schema_version":1,"required_paths":[null]}`},
		{"path number", `{"schema_version":1,"required_paths":[1]}`},
		{"path bool", `{"schema_version":1,"required_paths":[false]}`},
		{"path array", `{"schema_version":1,"required_paths":[[]]}`},
		{"empty path", `{"schema_version":1,"required_paths":[""]}`},
		{"absolute path", `{"schema_version":1,"required_paths":["/etc/agent"]}`},
		{"root path", `{"schema_version":1,"required_paths":["."]}`},
		{"dot segment", `{"schema_version":1,"required_paths":["etc/./agent"]}`},
		{"parent segment", `{"schema_version":1,"required_paths":["etc/../agent"]}`},
		{"repeated separator", `{"schema_version":1,"required_paths":["etc//agent"]}`},
		{"trailing separator", `{"schema_version":1,"required_paths":["etc/"]}`},
		{"negative limit", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":-1}`},
		{"limit overflow", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":9223372036854775808}`},
		{"limit negative overflow", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":-9223372036854775809}`},
		{"limit fraction", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":0.5}`},
		{"limit exponent", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":1e3}`},
		{"limit null", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":null}`},
		{"limit string", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":"5"}`},
		{"limit bool", `{"schema_version":1,"required_paths":[],"max_total_growth_bytes":false}`},
		{"trailing object", `{"schema_version":1,"required_paths":[]} {}`},
		{"trailing null", `{"schema_version":1,"required_paths":[]} null`},
		{"trailing garbage", `{"schema_version":1,"required_paths":[]}garbage`},
		{"BOM", "\xef\xbb\xbf" + `{"schema_version":1,"required_paths":[]}`},
		{"invalid UTF-8 path", "{\"schema_version\":1,\"required_paths\":[\"\xff\"]}"},
		{"invalid UTF-8 field", "{\"schema_version\":1,\"required_paths\":[],\"\xff\":0}"},
		{"unpaired high surrogate", `{"schema_version":1,"required_paths":["\ud800"]}`},
		{"unpaired low surrogate", `{"schema_version":1,"required_paths":["\udc00"]}`},
		{"high surrogate then character", `{"schema_version":1,"required_paths":["\ud800x"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rd.ParsePolicy(strings.NewReader(tc.input))
			if !errors.Is(err, rd.ErrInvalidInput) {
				t.Fatalf("ParsePolicy error = %v, want ErrInvalidInput", err)
			}
			var detail *rd.Error
			if !errors.As(err, &detail) || detail.Op == "" {
				t.Fatalf("invalid input error lacks typed operation context: %v", err)
			}
		})
	}
	if _, err := rd.ParsePolicy(nil); !errors.Is(err, rd.ErrInvalidInput) {
		t.Fatalf("nil reader error = %v, want ErrInvalidInput", err)
	}
}

type policyCountingReader struct {
	reader io.Reader
	n      int
}

func (r *policyCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.n += n
	return n, err
}

func TestParsePolicyByteLimit(t *testing.T) {
	const document = `{"schema_version":1,"required_paths":[]}`
	for _, length := range []int{policyInputLimit - 1, policyInputLimit} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			input := document + strings.Repeat(" ", length-len(document))
			policyParse(t, input)
		})
	}
	for _, length := range []int{policyInputLimit + 1, 2 * policyInputLimit} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			r := &policyCountingReader{reader: strings.NewReader(document + strings.Repeat(" ", length-len(document)))}
			_, err := rd.ParsePolicy(r)
			var limit *rd.LimitError
			if !errors.Is(err, rd.ErrLimitExceeded) || !errors.As(err, &limit) {
				t.Fatalf("oversized policy error = %v, want typed limit error", err)
			}
			if limit.Maximum != policyInputLimit || limit.Actual != policyInputLimit+1 || limit.Limit == "" {
				t.Fatalf("limit details = %#v, want maximum %d and first excess %d", limit, policyInputLimit, policyInputLimit+1)
			}
			if r.n > policyInputLimit+1 {
				t.Fatalf("read %d bytes, exceeding bounded lookahead", r.n)
			}
		})
	}
}

type policyErrorReader struct {
	data   []byte
	err    error
	closed bool
}

func (r *policyErrorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func (r *policyErrorReader) Close() error { r.closed = true; return nil }

func TestParsePolicyReaderErrors(t *testing.T) {
	failure := errors.New("policy reader failed")
	for _, data := range []string{"", `{"schema_version":`, `{"schema_version":1,"required_paths":[]}`, `{"schema_version":1,"required_paths":[]} `} {
		r := &policyErrorReader{data: []byte(data), err: failure}
		_, err := rd.ParsePolicy(r)
		if !errors.Is(err, failure) {
			t.Fatalf("data %q: error = %v, want underlying I/O failure", data, err)
		}
		if errors.Is(err, rd.ErrInvalidInput) {
			t.Fatalf("I/O failure was mislabeled invalid input: %v", err)
		}
		var detail *rd.Error
		if !errors.As(err, &detail) || detail.Op == "" {
			t.Fatalf("I/O failure lacks typed operation context: %v", err)
		}
		if r.closed {
			t.Fatal("ParsePolicy closed the caller-owned reader")
		}
	}
}

func TestEvaluatePresenceAndOrder(t *testing.T) {
	after := rd.Manifest{Entries: []rd.Entry{
		policyFile("z/file", 1),
		{Path: "var/log/link", Kind: rd.KindSymlink, LinkTarget: "/missing/../device", Mode: 0777},
		{Path: "explicit", Kind: rd.KindDirectory, Mode: 0700},
		{Path: "pipe", Kind: rd.KindFIFO},
	}}
	diff := policyCompare(t, rd.Manifest{}, after, rd.CompareOptions{IgnorePrefixes: []string{"var/log"}, IgnorePaths: []string{"absent"}})
	policy := policyParse(t, `{"schema_version":1,"required_paths":["z","z/file","var/log/link","explicit","pipe","absent","a","a"],"max_total_growth_bytes":0}`)
	got := policyEvaluate(t, after, diff, policy)
	want := []rd.Violation{
		{Rule: rd.RuleMaxTotalGrowthBytes, Limit: policyInt(0), Actual: policyInt(1)},
		{Rule: rd.RuleRequiredPath, Path: "a"},
		{Rule: rd.RuleRequiredPath, Path: "absent"},
		{Rule: rd.RuleRequiredPath, Path: "z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
	// Reordering explicit entries and requirements must not change the result.
	after.Entries[0], after.Entries[3] = after.Entries[3], after.Entries[0]
	policy.RequiredPaths = []string{"a", "pipe", "z/file", "z", "explicit", "absent", "var/log/link"}
	if repeated := policyEvaluate(t, after, diff, policy); !reflect.DeepEqual(repeated, want) {
		t.Fatalf("reordered input changed violations: %#v", repeated)
	}
}

func TestEvaluateGrowthBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after int64
		limit         *int64
		violates      bool
	}{
		{"omitted disables growth", 0, 12, nil, false},
		{"limit minus one", 100, 109, policyInt(10), false},
		{"limit equal", 100, 110, policyInt(10), false},
		{"limit plus one", 100, 111, policyInt(10), true},
		{"shrinking", 100, 1, policyInt(0), false},
		{"unchanged", 100, 100, policyInt(0), false},
		{"zero forbids positive", 0, 1, policyInt(0), true},
		{"zero baseline equal", 0, 10, policyInt(10), false},
		{"empty payload", 0, 0, policyInt(0), false},
		{"maximum growth equal", 0, math.MaxInt64, policyInt(math.MaxInt64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := rd.Manifest{Entries: []rd.Entry{policyFile("file", tc.before)}}
			after := rd.Manifest{Entries: []rd.Entry{policyFile("file", tc.after)}}
			diff := policyCompare(t, before, after, rd.CompareOptions{})
			got := policyEvaluate(t, after, diff, rd.Policy{SchemaVersion: 1, MaxTotalGrowthBytes: tc.limit})
			if !tc.violates {
				if len(got) != 0 {
					t.Fatalf("unexpected violations: %#v", got)
				}
				return
			}
			want := []rd.Violation{{Rule: rd.RuleMaxTotalGrowthBytes, Limit: tc.limit, Actual: policyInt(tc.after - tc.before)}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("violations = %#v, want %#v", got, want)
			}
		})
	}
}

func TestEvaluateUsesFilteredNetGrowth(t *testing.T) {
	before := rd.Manifest{Entries: []rd.Entry{policyFile("removed", 90), policyFile("ignored", 1)}}
	after := rd.Manifest{Entries: []rd.Entry{policyFile("added", 100), policyFile("ignored", 5000)}}
	diff := policyCompare(t, before, after, rd.CompareOptions{IgnorePaths: []string{"ignored"}})
	if diff.Summary.NetGrowthBytes != 10 || !reflect.DeepEqual(diff.PositiveGrowth, []rd.Growth{{Path: "added", Bytes: 100}}) {
		t.Fatalf("test comparison did not produce independent expected net +10 and positive +100: %#v", diff)
	}
	got := policyEvaluate(t, after, diff, rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"ignored"}, MaxTotalGrowthBytes: policyInt(10)})
	if len(got) != 0 {
		t.Fatalf("filtered net growth and ignored presence should pass: %#v", got)
	}
	got = policyEvaluate(t, after, diff, rd.Policy{SchemaVersion: 1, MaxTotalGrowthBytes: policyInt(9)})
	want := []rd.Violation{{Rule: rd.RuleMaxTotalGrowthBytes, Limit: policyInt(9), Actual: policyInt(10)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestEvaluateRejectsCallerBuiltInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		after  rd.Manifest
		diff   rd.Diff
		policy rd.Policy
	}{
		{"zero policy", rd.Manifest{}, rd.Diff{}, rd.Policy{}},
		{"unknown version", rd.Manifest{}, rd.Diff{}, rd.Policy{SchemaVersion: 2}},
		{"invalid policy path", rd.Manifest{}, rd.Diff{}, rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"../agent"}}},
		{"invalid UTF-8 policy path", rd.Manifest{}, rd.Diff{}, rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"\xff"}}},
		{"negative policy growth", rd.Manifest{}, rd.Diff{}, rd.Policy{SchemaVersion: 1, MaxTotalGrowthBytes: policyInt(-1)}},
		{"invalid candidate path", rd.Manifest{Entries: []rd.Entry{policyFile("../bad", 0)}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"invalid ignored candidate", rd.Manifest{Entries: []rd.Entry{{Path: "ignored", Kind: rd.KindRegular, SHA256: "bad"}}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"duplicate candidate", rd.Manifest{Entries: []rd.Entry{policyFile("file", 0), policyFile("file", 0)}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"candidate invalid ancestor", rd.Manifest{Entries: []rd.Entry{policyFile("file", 0), policyFile("file/child", 0)}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"candidate broken hardlink", rd.Manifest{Entries: []rd.Entry{{Path: "alias", Kind: rd.KindHardlink, SHA256: policyDigest, LinkTarget: "missing"}}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"candidate total overflow", rd.Manifest{Entries: []rd.Entry{policyFile("a", math.MaxInt64), policyFile("b", 1)}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"candidate invalid warning", rd.Manifest{Warnings: []rd.Warning{{Path: "missing", Code: rd.WarningUncomparedMetadata, Key: "vendor"}}}, rd.Diff{}, rd.Policy{SchemaVersion: 1}},
		{"negative diff count", rd.Manifest{}, rd.Diff{Summary: rd.Summary{Added: -1}}, rd.Policy{SchemaVersion: 1}},
		{"inconsistent diff growth", rd.Manifest{}, rd.Diff{Summary: rd.Summary{NetGrowthBytes: 1}}, rd.Policy{SchemaVersion: 1}},
		{"invalid diff change", rd.Manifest{}, rd.Diff{Changes: []rd.Change{{Path: "file", Kind: rd.ChangeAdded}}}, rd.Policy{SchemaVersion: 1}},
		{"invalid diff warning", rd.Manifest{}, rd.Diff{Warnings: []rd.Warning{{Path: "file", Code: rd.WarningUncomparedMetadata, Key: "vendor"}}}, rd.Policy{SchemaVersion: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rd.Evaluate(tc.after, tc.diff, tc.policy)
			if !errors.Is(err, rd.ErrInvalidInput) {
				t.Fatalf("Evaluate error = %v, want ErrInvalidInput", err)
			}
			if len(got) != 0 {
				t.Fatalf("invalid input returned partial violations: %#v", got)
			}
		})
	}
}

func TestEvaluateDoesNotMutateOrAliasInputs(t *testing.T) {
	after := rd.Manifest{
		Entries: []rd.Entry{policyFile("z", 4), policyFile("a", 3)},
		Warnings: []rd.Warning{
			{Path: "z", Code: rd.WarningUncomparedMetadata, Key: "vendor.z"},
			{Path: "a", Code: rd.WarningUncomparedMetadata, Key: "vendor.a"},
		},
	}
	diff := policyCompare(t, rd.Manifest{}, after, rd.CompareOptions{})
	policy := rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"z-missing", "a-missing", "z-missing"}, MaxTotalGrowthBytes: policyInt(1)}
	before, err := json.Marshal([]any{after, diff, policy})
	if err != nil {
		t.Fatal(err)
	}
	got := policyEvaluate(t, after, diff, policy)
	next, err := json.Marshal([]any{after, diff, policy})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, next) {
		t.Fatalf("Evaluate mutated input:\nbefore %s\nafter  %s", before, next)
	}
	if len(got) != 3 || got[0].Limit == nil || got[0].Actual == nil {
		t.Fatalf("expected growth plus two missing-path violations: %#v", got)
	}
	*got[0].Limit, *got[0].Actual = 100, 200
	got[1].Path = "changed"
	if *policy.MaxTotalGrowthBytes != 1 || diff.Summary.NetGrowthBytes != 7 {
		t.Fatal("returned violation aliases input numeric values")
	}
	want := []rd.Violation{
		{Rule: rd.RuleMaxTotalGrowthBytes, Limit: policyInt(1), Actual: policyInt(7)},
		{Rule: rd.RuleRequiredPath, Path: "a-missing"},
		{Rule: rd.RuleRequiredPath, Path: "z-missing"},
	}
	if repeated := policyEvaluate(t, after, diff, policy); !reflect.DeepEqual(repeated, want) {
		t.Fatalf("changing returned values affected a later evaluation: %#v", repeated)
	}
	*policy.MaxTotalGrowthBytes = 2
	if *want[0].Limit != 1 {
		t.Fatal("independent expected data unexpectedly changed")
	}
}
