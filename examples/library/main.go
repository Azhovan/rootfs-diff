// Command library demonstrates the public rootfsdiff and report packages using
// tiny in-memory tar inputs. It does not extract or execute archive contents.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	rootfsdiff "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/report"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, describeError(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, out io.Writer) error {
	// Archive creation makes the example runnable without external files. A
	// real consumer can pass opened tar/tar.gz files directly to Scan instead.
	baseline, err := makeArchive([]archiveEntry{
		{path: "etc/build-id", mode: 0644, contents: "build1"},
		{path: "etc/device.conf", mode: 0640, contents: "hello"},
		{path: "usr/bin/agent", mode: 0777, target: "../lib/agent-v1"},
	})
	if err != nil {
		return err
	}
	candidate, err := makeArchive([]archiveEntry{
		{path: "etc/build-id", mode: 0644, contents: "build-two"},
		{path: "etc/device.conf", mode: 06750, contents: "hello\n"},
		{path: "usr/bin/agent", mode: 0777, target: "../lib/agent-v2"},
	})
	if err != nil {
		return err
	}

	// Zero options select defaults. Readers belong to this caller; Scan never
	// closes them. bytes.Reader needs no Close. Close files in your application.
	before, err := rootfsdiff.Scan(ctx, bytes.NewReader(baseline), rootfsdiff.ScanOptions{})
	if err != nil {
		return fmt.Errorf("scan baseline: %w", err)
	}
	after, err := rootfsdiff.Scan(ctx, bytes.NewReader(candidate), rootfsdiff.ScanOptions{})
	if err != nil {
		return fmt.Errorf("scan candidate: %w", err)
	}
	diff, err := rootfsdiff.Compare(before, after, rootfsdiff.CompareOptions{
		IgnorePaths: []string{"etc/build-id"}, // Literal canonical path, not a glob.
	})
	if err != nil {
		return fmt.Errorf("compare: %w", err)
	}

	// ParsePolicy applies strict JSON checks beyond ordinary JSON decoding.
	policy, err := rootfsdiff.ParsePolicy(strings.NewReader(`{
		"schema_version": 1,
		"required_paths": ["etc/build-id", "usr/bin/agent"],
		"max_total_growth_bytes": 1
	}`))
	if err != nil {
		return fmt.Errorf("parse policy: %w", err)
	}
	// Evaluate uses the full candidate, so the ignored build ID still satisfies
	// required presence. Symlink presence does not verify its target. Exactly
	// one byte of retained net growth equals the policy budget and passes.
	violations, err := rootfsdiff.Evaluate(after, diff, policy)
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}
	result := rootfsdiff.Report{
		SchemaVersion:      rootfsdiff.SchemaVersion,
		Summary:            diff.Summary,
		Changes:            diff.Changes,
		PositiveGrowth:     diff.PositiveGrowth,
		Violations:         violations,
		Warnings:           diff.Warnings,
		MetadataExclusions: rootfsdiff.MetadataExclusions(),
	}
	// The caller chooses process gates. This example renders returned violations
	// as data; it does not implement the product CLI's exit-code policy.
	if err := report.Write(out, result, report.JSON); err != nil {
		// A failed writer can leave partial output. Never consume it as complete.
		return fmt.Errorf("render report: %w", err)
	}
	return nil
}

// describeError demonstrates category checks and typed context without losing
// underlying errors. Check cancellation before contextual *Error wrappers.
func describeError(err error) string {
	var limit *rootfsdiff.LimitError
	var input *rootfsdiff.Error
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled: " + err.Error()
	case errors.Is(err, rootfsdiff.ErrLimitExceeded) && errors.As(err, &limit):
		return fmt.Sprintf("%s exceeded: observed %d, maximum %d: %v", limit.Limit, limit.Actual, limit.Maximum, err)
	case errors.Is(err, rootfsdiff.ErrUnsupportedInput):
		return "unsupported input: " + err.Error()
	case errors.Is(err, rootfsdiff.ErrInvalidInput) && errors.As(err, &input):
		return fmt.Sprintf("invalid input during %s at archive path %q: %v", input.Op, input.Path, err)
	default:
		return err.Error() // Includes preserved reader/writer errors.
	}
}

type archiveEntry struct {
	path     string
	mode     int64
	contents string
	target   string
}

func makeArchive(entries []archiveEntry) ([]byte, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, entry := range entries {
		h := &tar.Header{
			Name: entry.path, Mode: entry.mode, Uid: 100, Gid: 200,
			Typeflag: tar.TypeReg, Size: int64(len(entry.contents)),
		}
		if entry.target != "" {
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, entry.target, 0
		}
		if err := w.WriteHeader(h); err != nil {
			return nil, fmt.Errorf("write example tar header: %w", err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := io.WriteString(w, entry.contents); err != nil {
				return nil, fmt.Errorf("write example tar payload: %w", err)
			}
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finish example tar: %w", err)
	}
	return buf.Bytes(), nil
}
