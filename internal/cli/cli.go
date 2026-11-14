// Package cli composes the library and report renderer for the rootfs-diff command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	rd "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/report"
)

// File is an input owned by Run. Run checks its mode and always closes it.
type File interface {
	io.ReadCloser
	Stat() (fs.FileInfo, error)
}

// Options supplies build metadata and the input opener. The zero value uses
// version "dev" and local files. OpenFile is injectable for I/O tests; it must
// return a fresh owned File on success and must not return a file with an error.
// The default opener rejects nonregular paths before opening them, so a FIFO
// cannot block an ordinary invocation. Opened files are also checked with Stat.
type Options struct {
	Version  string
	OpenFile func(string) (File, error)
}

// Run processes args (excluding argv[0]), writing reports/help/version to stdout
// and operational diagnostics to stderr. It returns 0 for successful checks, 1
// for configured gates, and 2 for usage, input, I/O, render or cancellation errors.
// Both writers must be non-nil. They are never closed or flushed. A writer error
// may leave partial output. Cancellation cannot interrupt a blocked Read/Write.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, opts Options) int {
	if stderr == nil {
		return 2
	}
	code, err := run(ctx, args, stdout, opts)
	if err != nil {
		if writeErr := writeString(stderr, fmt.Sprintf("rootfs-diff: %v\n", err)); writeErr != nil {
			return 2
		}
	}
	return code
}

func run(ctx context.Context, args []string, stdout io.Writer, opts Options) (int, error) {
	if ctx == nil {
		return 2, errors.New("nil context")
	}
	if stdout == nil {
		return 2, errors.New("nil stdout writer")
	}
	flags := flag.NewFlagSet("rootfs-diff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var format, policyPath string
	var help, version, failOnChange bool
	var scanOpts rd.ScanOptions
	var compareOpts rd.CompareOptions
	flags.StringVar(&format, "format", string(report.Text), "report format")
	flags.Func("policy", "strict JSON policy file", func(value string) error {
		if value == "" {
			return errors.New("policy path must not be empty")
		}
		policyPath = value
		return nil
	})
	flags.Func("ignore-path", "repeatable exact canonical path", func(value string) error {
		compareOpts.IgnorePaths = append(compareOpts.IgnorePaths, value)
		return nil
	})
	flags.Func("ignore-prefix", "repeatable canonical directory prefix", func(value string) error {
		compareOpts.IgnorePrefixes = append(compareOpts.IgnorePrefixes, value)
		return nil
	})
	flags.BoolVar(&failOnChange, "fail-on-change", false, "fail for any retained change")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&version, "version", false, "show version")
	for _, limit := range []struct {
		name  string
		value *int64
	}{
		{"max-entries", &scanOpts.MaxEntries},
		{"max-archive-bytes", &scanOpts.MaxArchiveBytes},
		{"max-file-bytes", &scanOpts.MaxFileBytes},
		{"max-path-bytes", &scanOpts.MaxPathBytes},
	} {
		flags.Var(decimalLimit{limit.value}, limit.name, "positive decimal integer")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return output(stdout, helpText)
		}
		return 2, fmt.Errorf("arguments: %w (use --help)", err)
	}
	if help {
		return output(stdout, helpText)
	}
	if version {
		buildVersion := opts.Version
		if buildVersion == "" {
			buildVersion = "dev"
		}
		return output(stdout, "rootfs-diff "+buildVersion+"\n")
	}
	paths := flags.Args()
	if len(paths) != 2 {
		return 2, errors.New("expected two local archive paths; flags must precede BEFORE and AFTER (use --help)")
	}
	for _, path := range paths {
		if err := localPath(path); err != nil {
			return 2, err
		}
	}
	if format != string(report.Text) && format != string(report.JSON) && format != string(report.Markdown) {
		return 2, fmt.Errorf("unsupported format %q; expected text, json or markdown", format)
	}
	if err := compareOpts.Validate(); err != nil {
		return 2, fmt.Errorf("ignore rules: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return 2, err
	}
	if opts.OpenFile == nil {
		opts.OpenFile = openLocalFile
	}
	var policy rd.Policy
	if policyPath != "" {
		if err := localPath(policyPath); err != nil {
			return 2, fmt.Errorf("policy: %w", err)
		}
		err := readFile(ctx, opts, policyPath, "policy", func(r io.Reader) error {
			var err error
			policy, err = rd.ParsePolicy(r)
			return err
		})
		if err != nil {
			return 2, err
		}
	}
	var manifests [2]rd.Manifest
	for i, path := range paths {
		role := []string{"before archive", "after archive"}[i]
		err := readFile(ctx, opts, path, role, func(r io.Reader) error {
			var err error
			manifests[i], err = rd.Scan(ctx, r, scanOpts)
			return err
		})
		if err != nil {
			return 2, err
		}
	}
	diff, err := rd.Compare(manifests[0], manifests[1], compareOpts)
	if err != nil {
		return 2, fmt.Errorf("compare: %w", err)
	}
	var violations []rd.Violation
	if policyPath != "" {
		violations, err = rd.Evaluate(manifests[1], diff, policy)
		if err != nil {
			return 2, fmt.Errorf("evaluate policy: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return 2, err
	}
	r := rd.Report{
		SchemaVersion: rd.SchemaVersion, Summary: diff.Summary,
		Changes: diff.Changes, PositiveGrowth: diff.PositiveGrowth,
		Violations: violations, Warnings: diff.Warnings,
		MetadataExclusions: rd.MetadataExclusions(),
	}
	if err := report.Write(stdout, r, report.Format(format)); err != nil {
		return 2, err
	}
	if err := ctx.Err(); err != nil {
		return 2, err
	}
	if len(violations) != 0 || (failOnChange && len(diff.Changes) != 0) {
		return 1, nil
	}
	return 0, nil
}

// Check path forms before touching the host. A leading '-' must be written with
// a local prefix such as './'; flags after positional arguments are invalid.
func localPath(path string) error {
	if path == "" || strings.HasPrefix(path, "-") || strings.Contains(path, "://") {
		return fmt.Errorf("expected a local file path, got %q; stdin/URLs are unsupported and flags must precede paths", path)
	}
	return nil
}

func openLocalFile(path string) (File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("expected a local regular file")
	}
	// Do not follow a final symlink or block if the file was replaced by a FIFO
	// between Lstat and open. readFile checks the opened descriptor's mode too.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func readFile(ctx context.Context, opts Options, path, role string, consume func(io.Reader) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := opts.OpenFile(path)
	if err != nil {
		return fmt.Errorf("open %s %q: %w", role, path, err)
	}
	if file == nil {
		return fmt.Errorf("open %s %q: nil file", role, path)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s %q: %w", role, path, closeErr))
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s %q: %w", role, path, err)
	}
	if info == nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s %q: expected a local regular file", role, path)
	}
	if err := consume(contextReader{ctx, file}); err != nil {
		return fmt.Errorf("read %s %q: %w", role, path, err)
	}
	return ctx.Err()
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

type decimalLimit struct{ value *int64 }

func (d decimalLimit) String() string { return strconv.FormatInt(*d.value, 10) }
func (d decimalLimit) Set(s string) error {
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return errors.New("limit must be a positive decimal integer")
	}
	value, err := strconv.ParseInt(s, 10, 64)
	if err != nil || value <= 0 {
		return errors.New("limit must be a positive decimal int64")
	}
	*d.value = value
	return nil
}

func writeString(w io.Writer, s string) error {
	n, err := io.WriteString(w, s)
	if err != nil {
		return err
	}
	if n != len(s) {
		return io.ErrShortWrite
	}
	return nil
}

func output(w io.Writer, text string) (int, error) {
	if err := writeString(w, text); err != nil {
		return 2, err
	}
	return 0, nil
}
