package synthetic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"time"

	rd "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/report"
)

// Check runs the actual compiled CLI on inputs made by Generate and checks its
// full structured output against independently specified expectations. It also
// checks the library on those bytes. The returned text report is for display.
func Check(binary, dir string) ([]byte, error) {
	path := func(name string) string { return filepath.Join(dir, name) }
	ignored := []string{"--ignore-path", "etc/demo.conf", "--ignore-path", "etc/current", "--ignore-path", "etc/init.d/S90demo", "--ignore-path", "usr/bin/demo-agent", "--ignore-prefix", "usr/share"}
	type comparison struct {
		name          string
		flags         []string
		before, after string
		code          int
		want          rd.Report
	}
	cases := []comparison{
		{"baseline itself", []string{"--fail-on-change"}, "before.tar", "before.tar", 0, sameReport(false)},
		{"baseline reordered gzip", []string{"--fail-on-change"}, "before.tar", "before-reordered.tar.gz", 0, sameReport(false)},
		{"candidate reordered gzip", []string{"--fail-on-change"}, "after.tar", "after-reordered.tar.gz", 0, sameReport(true)},
		{"ordinary differences", nil, "before.tar", "after.tar", 0, changedReport()},
		{"equivalent encoding", nil, "before-reordered.tar.gz", "after-reordered.tar.gz", 0, changedReport()},
		{"change gate", []string{"--fail-on-change"}, "before.tar", "after.tar", 1, changedReport()},
		{"policy gates", []string{"--policy", path("policy.json")}, "before.tar", "after.tar", 1, failingReport()},
		{"growth equality", []string{"--policy", path("equal-policy.json")}, "before.tar", "after.tar", 0, changedReport()},
		{"exact ignore", []string{"--ignore-path", "etc/demo.conf"}, "before.tar", "after.tar", 0, exactIgnoredReport()},
		{"prefix ignore and required path", []string{"--ignore-prefix", "usr/share", "--policy", path("policy.json")}, "before.tar", "after.tar", 1, prefixIgnoredReport()},
		{"all changes ignored", append([]string{"--fail-on-change"}, ignored...), "before.tar", "after.tar", 0, allIgnoredReport(false)},
		{"ignored missing required path", append([]string{"--policy", path("policy.json")}, ignored...), "before.tar", "after.tar", 1, allIgnoredReport(true)},
	}
	var ordinary []byte
	for _, c := range cases {
		args := append([]string{"--format", "json"}, c.flags...)
		args = append(args, path(c.before), path(c.after))
		out, diagnostic, code, err := invoke(binary, args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.name, err)
		}
		if code != c.code || len(diagnostic) != 0 {
			return nil, fmt.Errorf("%s: exit %d, want %d; stderr: %s", c.name, code, c.code, diagnostic)
		}
		if err := equalJSON(out, c.want); err != nil {
			return nil, fmt.Errorf("%s: %w", c.name, err)
		}
		if c.name == "ordinary differences" {
			ordinary = out
		}
		if c.name == "equivalent encoding" || c.name == "change gate" || c.name == "growth equality" {
			if !bytes.Equal(out, ordinary) {
				return nil, fmt.Errorf("%s: logically identical report bytes differ", c.name)
			}
		}
	}
	for _, format := range []string{"text", "json", "markdown"} {
		args := []string{"--format", format, path("before.tar"), path("after.tar")}
		first, diagnostic, code, err := invoke(binary, args...)
		if err != nil || code != 0 || len(diagnostic) != 0 {
			return nil, fmt.Errorf("%s initial report: exit %d, stderr %q, error %v", format, code, diagnostic, err)
		}
		second, diagnostic, code, err := invoke(binary, args...)
		if err != nil || code != 0 || len(diagnostic) != 0 || !bytes.Equal(first, second) {
			return nil, fmt.Errorf("%s repeat report differs: exit %d, stderr %q, error %v", format, code, diagnostic, err)
		}
		for _, name := range []string{"before.tar", "after.tar"} {
			if bytes.Contains(first, []byte(path(name))) {
				return nil, fmt.Errorf("%s report contains an absolute input filename", format)
			}
		}
	}
	for _, c := range []struct {
		name  string
		flags []string
		after string
	}{
		{"unsafe path", nil, "unsafe.tar"},
		{"truncated header", nil, "truncated.tar"},
		{"corrupt gzip checksum", nil, "corrupt.tar.gz"},
		{"invalid policy", []string{"--policy", path("invalid-policy.json")}, "after.tar"},
		{"file limit", []string{"--max-file-bytes", "1"}, "after.tar"},
	} {
		args := append([]string{"--format", "json"}, c.flags...)
		args = append(args, path("before.tar"), path(c.after))
		out, diagnostic, code, err := invoke(binary, args...)
		if err != nil || code != 2 || len(out) != 0 || len(diagnostic) == 0 {
			return nil, fmt.Errorf("%s: want exit 2 with empty stdout and diagnostic; exit %d stdout %q stderr %q error %v", c.name, code, out, diagnostic, err)
		}
	}
	if err := checkLibrary(dir, ordinary); err != nil {
		return nil, err
	}
	out, diagnostic, code, err := invoke(binary, "--policy", path("policy.json"), path("before.tar"), path("after.tar"))
	if err != nil || code != 1 || len(diagnostic) != 0 {
		return nil, fmt.Errorf("readable gated report: exit %d, stderr %q, error %v", code, diagnostic, err)
	}
	return out, nil
}

func invoke(binary string, args ...string) ([]byte, []byte, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	var out, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, nil, 0, ctx.Err()
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, nil, 0, err
		}
		code = exit.ExitCode()
	}
	return out.Bytes(), diagnostic.Bytes(), code, nil
}

func equalJSON(got []byte, want any) error {
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		return fmt.Errorf("invalid report JSON: %w", err)
	}
	b, err := json.Marshal(want)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("report differs from independent expectation\nwant: %s\ngot: %s", b, got)
	}
	return nil
}

func checkLibrary(dir string, cliReport []byte) error {
	var manifests [2]rd.Manifest
	for i, name := range []string{"before.tar", "after.tar"} {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		manifests[i], err = rd.Scan(context.Background(), file, rd.ScanOptions{})
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return fmt.Errorf("library scan %s: %w", name, errors.Join(err, closeErr))
		}
	}
	for i, expected := range []rd.Manifest{beforeManifest(), afterManifest()} {
		if err := equalModel(manifests[i], expected); err != nil {
			return fmt.Errorf("independent library manifest %d: %w", i, err)
		}
	}
	diff, err := rd.Compare(manifests[0], manifests[1], rd.CompareOptions{})
	if err != nil {
		return err
	}
	r := rd.Report{SchemaVersion: 1, Summary: diff.Summary, Changes: diff.Changes, PositiveGrowth: diff.PositiveGrowth,
		Warnings: diff.Warnings, MetadataExclusions: rd.MetadataExclusions()}
	var library bytes.Buffer
	if err := report.Write(&library, r, report.JSON); err != nil {
		return err
	}
	if err := equalJSON(library.Bytes(), changedReport()); err != nil {
		return fmt.Errorf("library report: %w", err)
	}
	if !bytes.Equal(library.Bytes(), cliReport) {
		return errors.New("library and CLI JSON bytes differ")
	}
	file, err := os.Open(filepath.Join(dir, "policy.json"))
	if err != nil {
		return err
	}
	policy, err := rd.ParsePolicy(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	r.Violations, err = rd.Evaluate(manifests[1], diff, policy)
	if err != nil {
		return err
	}
	if err := equalModel(r, failingReport()); err != nil {
		return fmt.Errorf("library policy: %w", err)
	}
	return nil
}

func equalModel(got, want any) error {
	b, err := json.Marshal(got)
	if err != nil {
		return err
	}
	return equalJSON(b, want)
}
