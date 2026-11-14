package cli_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	rd "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/internal/testfixture"
)

// These tests execute the built command, including main's status selection and
// stream wiring. Expected models below are authored independently of Scan and
// Compare; those library results must agree with the same expectations.
func TestCLIExecutable(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	binary := buildCLI(t, root, "")
	fixture := newProcessFixture(t)

	t.Run("reports and gates", func(t *testing.T) {
		passing := fixture.policy(t, `{"schema_version":1,"required_paths":["etc/config","var/log/boot"],"max_total_growth_bytes":3}`)
		failing := fixture.policy(t, `{"schema_version":1,"required_paths":["missing"],"max_total_growth_bytes":2}`)
		ignoredPresent := fixture.policy(t, `{"schema_version":1,"required_paths":["var/log/boot"],"max_total_growth_bytes":2}`)
		ignoredMissing := fixture.policy(t, `{"schema_version":1,"required_paths":["var/log/missing"]}`)
		implicitParent := fixture.policy(t, `{"schema_version":1,"required_paths":["var/log"]}`)
		failureReport := expectedProcessReport()
		failureReport.Violations = []rd.Violation{
			{Rule: rd.RuleMaxTotalGrowthBytes, Limit: processInt64(2), Actual: processInt64(3)},
			{Rule: rd.RuleRequiredPath, Path: "missing"},
		}
		ignoredMissingReport := expectedWithoutLogs()
		ignoredMissingReport.Violations = []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "var/log/missing"}}
		implicitParentReport := expectedWithoutLogs()
		implicitParentReport.Violations = []rd.Violation{{Rule: rd.RuleRequiredPath, Path: "var/log"}}
		cases := []struct {
			name   string
			flags  []string
			before string
			after  string
			code   int
			want   rd.Report
		}{
			{"same input", nil, fixture.before, fixture.before, 0, expectedSameProcessReport()},
			{"compression and suffix do not change contents", nil, fixture.before, fixture.sameGzip, 0, expectedSameProcessReport()},
			{"ordinary differences succeed", nil, fixture.before, fixture.after, 0, expectedProcessReport()},
			{"passing policy", []string{"--policy", passing}, fixture.before, fixture.after, 0, expectedProcessReport()},
			{"failing policy complete report", []string{"--policy", failing}, fixture.before, fixture.after, 1, failureReport},
			{"change gate complete report", []string{"--fail-on-change"}, fixture.before, fixture.after, 1, expectedProcessReport()},
			{"change gate unchanged", []string{"--fail-on-change"}, fixture.before, fixture.before, 0, expectedSameProcessReport()},
			{"exact ignore retains other changes and warnings", []string{"--ignore-path", "etc/config"}, fixture.before, fixture.after, 0, expectedWithoutConfig()},
			{"prefix ignore", []string{"--ignore-prefix", "var/log"}, fixture.before, fixture.after, 0, expectedWithoutLogs()},
			{"repeatable ignores satisfy change gate", []string{"--fail-on-change", "--ignore-path", "added", "--ignore-path", "obsolete", "--ignore-path", "etc/config", "--ignore-path", "link", "--ignore-prefix", "var/log", "--ignore-prefix", "absent"}, fixture.before, fixture.after, 0, expectedAllChangesIgnored()},
			{"ignored required entry is present", []string{"--ignore-prefix", "var/log", "--policy", ignoredPresent}, fixture.before, fixture.after, 0, expectedWithoutLogs()},
			{"ignored missing required entry still fails", []string{"--ignore-prefix", "var/log", "--policy", ignoredMissing}, fixture.before, fixture.after, 1, ignoredMissingReport},
			{"implicit required parent is absent", []string{"--ignore-prefix", "var/log", "--policy", implicitParent}, fixture.before, fixture.after, 1, implicitParentReport},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				args := append([]string{"--format", "json"}, tc.flags...)
				args = append(args, tc.before, tc.after)
				got := invokeCLI(t, binary, args...)
				assertProcessStatus(t, got, tc.code)
				if len(got.stderr) != 0 {
					t.Fatalf("unexpected stderr: %s", got.stderr)
				}
				assertProcessReport(t, got.stdout, tc.want)
			})
		}

		t.Run("deterministic output", func(t *testing.T) {
			args := []string{"--format", "json", fixture.before, fixture.after}
			first, second := invokeCLI(t, binary, args...), invokeCLI(t, binary, args...)
			assertProcessStatus(t, first, 0)
			assertProcessStatus(t, second, 0)
			if !bytes.Equal(first.stdout, second.stdout) || len(first.stderr) != 0 || len(second.stderr) != 0 {
				t.Fatal("repeated equivalent invocations differ")
			}
			for _, hostPath := range []string{fixture.before, fixture.after, "metadata-value-must-not-appear"} {
				if bytes.Contains(first.stdout, []byte(hostPath)) {
					t.Fatalf("report disclosed host path or metadata value %q", hostPath)
				}
			}
		})

		t.Run("independent library agreement", func(t *testing.T) {
			before, err := rd.Scan(context.Background(), bytes.NewReader(fixture.beforeBytes), rd.ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}
			after, err := rd.Scan(context.Background(), bytes.NewReader(fixture.afterBytes), rd.ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}
			wantBefore, wantAfter := expectedProcessManifests()
			if !reflect.DeepEqual(before, wantBefore) || !reflect.DeepEqual(after, wantAfter) {
				t.Fatalf("library manifests differ from independent fixture metadata:\nbefore %#v\nafter %#v", before, after)
			}
			diff, err := rd.Compare(before, after, rd.CompareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := expectedProcessReport()
			wantDiff := rd.Diff{Summary: want.Summary, Changes: want.Changes, PositiveGrowth: want.PositiveGrowth, Warnings: want.Warnings}
			if !reflect.DeepEqual(diff, wantDiff) {
				t.Fatalf("library diff differs from independent report: %#v", diff)
			}
			policy := rd.Policy{SchemaVersion: 1, RequiredPaths: []string{"missing"}, MaxTotalGrowthBytes: processInt64(2)}
			violations, err := rd.Evaluate(after, diff, policy)
			if err != nil || !reflect.DeepEqual(violations, failureReport.Violations) {
				t.Fatalf("library violations = %#v, %v; want %#v", violations, err, failureReport.Violations)
			}
		})
	})

	t.Run("all output formats", func(t *testing.T) {
		for _, format := range []string{"", "text", "json", "markdown"} {
			t.Run("format="+format, func(t *testing.T) {
				var args []string
				if format != "" {
					args = []string{"--format", format}
				}
				got := invokeCLI(t, binary, append(args, fixture.before, fixture.after)...)
				assertProcessStatus(t, got, 0)
				if len(got.stderr) != 0 {
					t.Fatalf("unexpected stderr: %s", got.stderr)
				}
				if format == "json" {
					assertProcessReport(t, got.stdout, expectedProcessReport())
					return
				}
				heading := "rootfs-diff report (schema 1)\n"
				if format == "markdown" {
					heading = "# rootfs-diff report (schema 1)\n"
				}
				if !bytes.HasPrefix(got.stdout, []byte(heading)) {
					t.Fatalf("incorrect report heading: %s", got.stdout)
				}
				warningKey := "VENDOR.note"
				if format == "markdown" {
					warningKey = "VENDOR&#46;note"
				}
				for _, text := range []string{"added 1, removed 1, modified 3, unchanged 1", "before 5 bytes, after 8 bytes, net +3 bytes", "06750", "Metadata exclusions", warningKey} {
					if !bytes.Contains(got.stdout, []byte(text)) {
						t.Fatalf("report lacks %q: %s", text, got.stdout)
					}
				}
			})
		}
	})

	t.Run("limits", func(t *testing.T) {
		for _, tc := range []struct{ flag, value, diagnostic string }{
			{"--max-entries", "1", "max_entries"},
			{"--max-archive-bytes", "511", "max_archive_bytes"},
			{"--max-file-bytes", "2", "max_file_bytes"},
			{"--max-path-bytes", "4", "max_path_bytes"},
		} {
			t.Run(tc.flag, func(t *testing.T) {
				got := invokeCLI(t, binary, "--format", "json", tc.flag, tc.value, fixture.before, fixture.after)
				assertProcessFailure(t, got)
				if !bytes.Contains(got.stderr, []byte(tc.diagnostic)) {
					t.Fatalf("diagnostic does not identify limit %q: %s", tc.diagnostic, got.stderr)
				}
			})
			for _, invalid := range []string{"0", "-1", "1.5", "0x10", "", "9223372036854775808"} {
				t.Run(tc.flag+"="+invalid, func(t *testing.T) {
					got := invokeCLI(t, binary, "--format", "json", tc.flag+"="+invalid, fixture.before, fixture.after)
					assertProcessFailure(t, got)
				})
			}
		}
		got := invokeCLI(t, binary, "--format", "json", "--max-entries", "0005", "--max-archive-bytes", strconv.Itoa(max(len(fixture.beforeBytes), len(fixture.afterBytes))), "--max-file-bytes", "5", "--max-path-bytes", "12", fixture.before, fixture.after)
		assertProcessStatus(t, got, 0)
		if len(got.stderr) != 0 {
			t.Fatalf("valid positive overrides wrote stderr: %s", got.stderr)
		}
		assertProcessReport(t, got.stdout, expectedProcessReport())
	})

	t.Run("invalid arguments and files", func(t *testing.T) {
		missing := filepath.Join(fixture.dir, "missing.tar")
		fifo := filepath.Join(fixture.dir, "input.fifo")
		if err := syscall.Mkfifo(fifo, 0600); err != nil {
			t.Fatal(err)
		}
		symlink := filepath.Join(fixture.dir, "input-symlink.tar")
		if err := os.Symlink(fixture.before, symlink); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name string
			args []string
		}{
			{"missing both paths", nil},
			{"missing after path", []string{fixture.before}},
			{"extra path", []string{fixture.before, fixture.after, fixture.before}},
			{"flag after paths", []string{fixture.before, fixture.after, "--format=json"}},
			{"flag between paths", []string{fixture.before, "--format=json", fixture.after}},
			{"invalid flag", []string{"--future", fixture.before, fixture.after}},
			{"missing flag value", []string{"--format"}},
			{"invalid format", []string{"--format=xml", fixture.before, fixture.after}},
			{"invalid boolean", []string{"--fail-on-change=maybe", fixture.before, fixture.after}},
			{"invalid exact ignore", []string{"--ignore-path", "../bad", fixture.before, fixture.after}},
			{"invalid prefix ignore", []string{"--ignore-prefix", "var//log", fixture.before, fixture.after}},
			{"empty ignore", []string{"--ignore-path=", fixture.before, fixture.after}},
			{"empty policy option", []string{"--policy=", fixture.before, fixture.after}},
			{"stdin unsupported", []string{"-", fixture.after}},
			{"URL unsupported", []string{"https://example.invalid/input.tar", fixture.after}},
			{"policy stdin unsupported", []string{"--policy=-", fixture.before, fixture.after}},
			{"missing before file", []string{missing, fixture.after}},
			{"missing after file", []string{fixture.before, missing}},
			{"missing policy file", []string{"--policy", missing, fixture.before, fixture.after}},
			{"directory input", []string{fixture.dir, fixture.after}},
			{"FIFO input rejected before blocking open", []string{fifo, fixture.after}},
			{"symlink archive input rejected", []string{symlink, fixture.after}},
			{"symlink policy input rejected", []string{"--policy", symlink, fixture.before, fixture.after}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertProcessFailure(t, invokeCLI(t, binary, tc.args...))
			})
		}
		for name, policy := range map[string]string{
			"malformed":     `{`,
			"unknown field": `{"schema_version":1,"required_paths":[],"unexpected":0}`,
			"duplicate key": `{"schema_version":1,"schema_version":1,"required_paths":[]}`,
			"trailing JSON": `{"schema_version":1,"required_paths":[]} {}`,
		} {
			t.Run("policy "+name, func(t *testing.T) {
				path := fixture.policy(t, policy)
				got := invokeCLI(t, binary, "--format", "json", "--policy", path, fixture.before, fixture.after)
				assertProcessFailure(t, got)
			})
		}
	})

	t.Run("corrupt and unsupported archives emit no report", func(t *testing.T) {
		checksum := append([]byte(nil), fixture.afterGzip...)
		checksum[len(checksum)-8] ^= 0x80
		unsupported, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{Name: "unsupported", Typeflag: tar.TypeCont, Mode: 0644}})
		if err != nil {
			t.Fatal(err)
		}
		payloadArchive, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{Name: "payload", Typeflag: tar.TypeReg, Mode: 0644, Size: 5}, Data: []byte("hello")})
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name string
			data []byte
		}{
			{"malformed header", bytes.Repeat([]byte{'x'}, 512)},
			{"truncated header", fixture.beforeBytes[:100]},
			{"truncated payload", payloadArchive[:514]},
			{"bad gzip checksum", checksum},
			{"truncated gzip trailer", fixture.afterGzip[:len(fixture.afterGzip)-3]},
			{"unsupported entry", unsupported},
		} {
			t.Run(tc.name, func(t *testing.T) {
				path := fixture.write(t, tc.name+".tar", tc.data)
				for _, paths := range [][2]string{{path, fixture.after}, {fixture.before, path}} {
					got := invokeCLI(t, binary, "--format", "json", paths[0], paths[1])
					assertProcessFailure(t, got)
				}
			})
		}
	})

	t.Run("help and version without inputs", func(t *testing.T) {
		got := invokeCLI(t, binary, "--help")
		assertProcessStatus(t, got, 0)
		if len(got.stderr) != 0 || !bytes.HasPrefix(got.stdout, []byte("Usage: rootfs-diff [flags]")) {
			t.Fatalf("help stdout=%q stderr=%q", got.stdout, got.stderr)
		}
		for _, option := range []string{"--format", "--policy", "--ignore-path", "--ignore-prefix", "--fail-on-change", "--max-entries", "--max-archive-bytes", "--max-file-bytes", "--max-path-bytes", "--help", "--version"} {
			if !bytes.Contains(got.stdout, []byte(option)) {
				t.Errorf("help lacks %s", option)
			}
		}
		got = invokeCLI(t, binary, "--version")
		assertProcessStatus(t, got, 0)
		if string(got.stdout) != "rootfs-diff dev\n" || len(got.stderr) != 0 {
			t.Fatalf("development version stdout=%q stderr=%q", got.stdout, got.stderr)
		}
		versioned := buildCLI(t, root, "0.1.0-test")
		got = invokeCLI(t, versioned, "--version")
		assertProcessStatus(t, got, 0)
		if string(got.stdout) != "rootfs-diff 0.1.0-test\n" || len(got.stderr) != 0 {
			t.Fatalf("injected version stdout=%q stderr=%q", got.stdout, got.stderr)
		}
	})

	t.Run("closed downstream pipe is exit two", func(t *testing.T) {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		defer writer.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "--format", "json", fixture.before, fixture.after)
		cmd.Stdout = writer
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		if ctx.Err() != nil {
			t.Fatalf("closed-pipe invocation did not finish: %v", ctx.Err())
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 || !strings.Contains(stderr.String(), "broken pipe") {
			t.Fatalf("closed pipe error=%v stderr=%q; want exit 2 and I/O diagnostic", err, stderr.String())
		}
	})
}

type processResult struct {
	code           int
	stdout, stderr []byte
}

func buildCLI(t *testing.T, root, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "rootfs-diff")
	args := []string{"build", "-o", binary}
	if version != "" {
		args = append(args, "-ldflags", "-X main.version="+version)
	}
	args = append(args, "./cmd/rootfs-diff")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v\n%s", err, output)
	}
	return binary
}

func invokeCLI(t *testing.T, binary string, args ...string) processResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI did not finish for %q: %v", args, ctx.Err())
	}
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("start CLI: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return processResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes()}
}

func assertProcessStatus(t *testing.T, got processResult, want int) {
	t.Helper()
	if got.code != want {
		t.Fatalf("exit=%d; want %d\nstdout=%s\nstderr=%s", got.code, want, got.stdout, got.stderr)
	}
}

func assertProcessFailure(t *testing.T, got processResult) {
	t.Helper()
	assertProcessStatus(t, got, 2)
	if len(got.stdout) != 0 {
		t.Fatalf("pre-render failure emitted report bytes: %q", got.stdout)
	}
	if !bytes.HasPrefix(got.stderr, []byte("rootfs-diff: ")) {
		t.Fatalf("missing operational diagnostic: %q", got.stderr)
	}
}

func assertProcessReport(t *testing.T, data []byte, want rd.Report) {
	t.Helper()
	var got rd.Report
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&got); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, data)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("report is incomplete or invalid: %v\n%s", err, data)
	}
	// Marshal the independently expected model only to normalize nil versus
	// empty slices, whose wire representation is deliberately the same array.
	expectedJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var normalized rd.Report
	if err := json.Unmarshal(expectedJSON, &normalized); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, normalized) {
		t.Fatalf("report differs from independent expectation\ngot:  %s\nwant: %s", data, expectedJSON)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 7 {
		t.Fatalf("report has %d top-level fields, want all 7", len(raw))
	}
	for _, name := range []string{"schema_version", "summary", "changes", "positive_growth", "violations", "warnings", "metadata_exclusions"} {
		if _, present := raw[name]; !present {
			t.Errorf("report omits %s", name)
		}
	}
	for _, name := range []string{"changes", "positive_growth", "violations", "warnings", "metadata_exclusions"} {
		value := bytes.TrimSpace(raw[name])
		if len(value) == 0 || value[0] != '[' {
			t.Errorf("%s must be present as an array, got %q", name, value)
		}
	}
	var changes []map[string]json.RawMessage
	if err := json.Unmarshal(raw["changes"], &changes); err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		var kind string
		if err := json.Unmarshal(change["kind"], &kind); err != nil {
			t.Fatal(err)
		}
		if kind == "added" && string(change["before"]) != "null" {
			t.Error("added before side must be explicit null")
		}
		if kind == "removed" && string(change["after"]) != "null" {
			t.Error("removed after side must be explicit null")
		}
		fields := bytes.TrimSpace(change["changed_fields"])
		if len(fields) == 0 || fields[0] != '[' {
			t.Error("changed_fields must be present as an array")
		}
	}
	var violations []map[string]json.RawMessage
	if err := json.Unmarshal(raw["violations"], &violations); err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		if len(violation) != 4 {
			t.Error("violation must include rule, path, limit and actual")
		}
		if string(violation["rule"]) == `"required_path"` && (string(violation["limit"]) != "null" || string(violation["actual"]) != "null") {
			t.Error("required-path violation numbers must be explicit null")
		}
	}
}

type processFixture struct {
	dir, before, after, sameGzip       string
	beforeBytes, afterBytes, afterGzip []byte
}

func newProcessFixture(t *testing.T) processFixture {
	t.Helper()
	f := processFixture{dir: t.TempDir()}
	regular := func(path, data string, mode int64) testfixture.Entry {
		return testfixture.Entry{Header: tar.Header{Name: path, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(data))}, Data: []byte(data)}
	}
	config := func(data string, mode int64) testfixture.Entry {
		entry := regular("etc/config", data, mode)
		entry.Header.Uid, entry.Header.Gid = 12, 34
		entry.Header.Format = tar.FormatPAX
		entry.Header.PAXRecords = map[string]string{"VENDOR.note": "metadata-value-must-not-appear"}
		return entry
	}
	link := func(target string) testfixture.Entry {
		return testfixture.Entry{Header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Mode: 0777, Linkname: target}}
	}
	directory := testfixture.Entry{Header: tar.Header{Name: "usr", Typeflag: tar.TypeDir, Mode: 0755}}
	var err error
	f.beforeBytes, err = testfixture.Tar(regular("var/log/boot", "x", 0600), config("a", 0640), regular("obsolete", "bye", 0644), link("../old"), directory)
	if err != nil {
		t.Fatal(err)
	}
	f.afterBytes, err = testfixture.Tar(directory, link("../new"), regular("added", "z", 0644), config("hello", 06750), regular("var/log/boot", "xx", 0600))
	if err != nil {
		t.Fatal(err)
	}
	f.afterGzip, err = testfixture.Gzip(f.afterBytes)
	if err != nil {
		t.Fatal(err)
	}
	sameGzip, err := testfixture.Gzip(f.beforeBytes)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately misleading suffixes prove gzip detection uses bytes.
	f.before = f.write(t, "before.tar.gz", f.beforeBytes)
	f.after = f.write(t, "after.tar", f.afterGzip)
	f.sameGzip = f.write(t, "same.data", sameGzip)
	return f
}

func (f processFixture) write(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(f.dir, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f processFixture) policy(t *testing.T, text string) string {
	t.Helper()
	file, err := os.CreateTemp(f.dir, "policy-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func processEntry(path, data string, mode uint32) rd.Entry {
	digest := sha256.Sum256([]byte(data))
	return rd.Entry{Path: path, Kind: rd.KindRegular, Mode: mode, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:])}
}

func expectedProcessManifests() (rd.Manifest, rd.Manifest) {
	oldConfig, newConfig := processEntry("etc/config", "a", 0640), processEntry("etc/config", "hello", 06750)
	oldConfig.UID, oldConfig.GID, newConfig.UID, newConfig.GID = 12, 34, 12, 34
	before := rd.Manifest{
		Entries: []rd.Entry{
			oldConfig,
			{Path: "link", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../old"},
			processEntry("obsolete", "bye", 0644),
			{Path: "usr", Kind: rd.KindDirectory, Mode: 0755},
			processEntry("var/log/boot", "x", 0600),
		},
		Warnings: []rd.Warning{{Path: "etc/config", Code: rd.WarningUncomparedMetadata, Key: "VENDOR.note"}},
	}
	after := rd.Manifest{
		Entries: []rd.Entry{
			processEntry("added", "z", 0644),
			newConfig,
			{Path: "link", Kind: rd.KindSymlink, Mode: 0777, LinkTarget: "../new"},
			{Path: "usr", Kind: rd.KindDirectory, Mode: 0755},
			processEntry("var/log/boot", "xx", 0600),
		},
		Warnings: []rd.Warning{{Path: "etc/config", Code: rd.WarningUncomparedMetadata, Key: "VENDOR.note"}},
	}
	return before, after
}

func processExclusions() []string {
	return []string{"acls", "directory_sizes", "file_capabilities", "selinux_labels", "timestamps", "unrecognized_pax_metadata", "user_group_names", "xattrs"}
}

func processWarnings() []rd.Warning {
	return []rd.Warning{
		{Source: rd.SourceAfter, Path: "etc/config", Code: rd.WarningUncomparedMetadata, Key: "VENDOR.note"},
		{Source: rd.SourceBefore, Path: "etc/config", Code: rd.WarningUncomparedMetadata, Key: "VENDOR.note"},
	}
}

func expectedProcessReport() rd.Report {
	before, after := expectedProcessManifests()
	return rd.Report{
		SchemaVersion: 1,
		Summary:       rd.Summary{BeforeEntries: 5, AfterEntries: 5, Added: 1, Removed: 1, Modified: 3, Unchanged: 1, BeforePayloadBytes: 5, AfterPayloadBytes: 8, NetGrowthBytes: 3},
		Changes: []rd.Change{
			{Path: "added", Kind: rd.ChangeAdded, After: &after.Entries[0], ChangedFields: []rd.Field{}},
			{Path: "etc/config", Kind: rd.ChangeModified, Before: &before.Entries[0], After: &after.Entries[1], ChangedFields: []rd.Field{rd.FieldMode, rd.FieldSize, rd.FieldSHA256}},
			{Path: "link", Kind: rd.ChangeModified, Before: &before.Entries[1], After: &after.Entries[2], ChangedFields: []rd.Field{rd.FieldLinkTarget}},
			{Path: "obsolete", Kind: rd.ChangeRemoved, Before: &before.Entries[2], ChangedFields: []rd.Field{}},
			{Path: "var/log/boot", Kind: rd.ChangeModified, Before: &before.Entries[4], After: &after.Entries[4], ChangedFields: []rd.Field{rd.FieldSize, rd.FieldSHA256}},
		},
		PositiveGrowth: []rd.Growth{{Path: "etc/config", Bytes: 4}, {Path: "added", Bytes: 1}, {Path: "var/log/boot", Bytes: 1}},
		Warnings:       processWarnings(), MetadataExclusions: processExclusions(),
	}
}

func expectedSameProcessReport() rd.Report {
	return rd.Report{
		SchemaVersion: 1,
		Summary:       rd.Summary{BeforeEntries: 5, AfterEntries: 5, Unchanged: 5, BeforePayloadBytes: 5, AfterPayloadBytes: 5},
		Warnings:      processWarnings(), MetadataExclusions: processExclusions(),
	}
}

func expectedWithoutConfig() rd.Report {
	r := expectedProcessReport()
	r.Summary = rd.Summary{BeforeEntries: 4, AfterEntries: 4, Added: 1, Removed: 1, Modified: 2, Unchanged: 1, BeforePayloadBytes: 4, AfterPayloadBytes: 3, NetGrowthBytes: -1}
	r.Changes = []rd.Change{r.Changes[0], r.Changes[2], r.Changes[3], r.Changes[4]}
	r.PositiveGrowth = []rd.Growth{{Path: "added", Bytes: 1}, {Path: "var/log/boot", Bytes: 1}}
	return r
}

func expectedWithoutLogs() rd.Report {
	r := expectedProcessReport()
	r.Summary = rd.Summary{BeforeEntries: 4, AfterEntries: 4, Added: 1, Removed: 1, Modified: 2, Unchanged: 1, BeforePayloadBytes: 4, AfterPayloadBytes: 6, NetGrowthBytes: 2}
	r.Changes = []rd.Change{r.Changes[0], r.Changes[1], r.Changes[2], r.Changes[3]}
	r.PositiveGrowth = []rd.Growth{{Path: "etc/config", Bytes: 4}, {Path: "added", Bytes: 1}}
	return r
}

func expectedAllChangesIgnored() rd.Report {
	return rd.Report{
		SchemaVersion: 1, Summary: rd.Summary{BeforeEntries: 1, AfterEntries: 1, Unchanged: 1},
		Warnings: processWarnings(), MetadataExclusions: processExclusions(),
	}
}

func processInt64(value int64) *int64 { return &value }
