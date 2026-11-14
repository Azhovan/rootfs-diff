package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Azhovan/rootfs-diff/internal/testfixture"
)

func emptyArchive(t *testing.T) []byte {
	t.Helper()
	data, err := testfixture.Tar()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type inputFile struct {
	reader            io.Reader
	mode              fs.FileMode
	statErr, closeErr error
	closed            int
}

func (f *inputFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *inputFile) Close() error               { f.closed++; return f.closeErr }
func (f *inputFile) Stat() (fs.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return fileInfo{f.mode}, nil
}

type fileInfo struct{ mode fs.FileMode }

func (fileInfo) Name() string        { return "input" }
func (fileInfo) Size() int64         { return 0 }
func (f fileInfo) Mode() fs.FileMode { return f.mode }
func (fileInfo) ModTime() time.Time  { return time.Time{} }
func (f fileInfo) IsDir() bool       { return f.mode.IsDir() }
func (fileInfo) Sys() any            { return nil }

type readFailure struct{ err error }

func (r readFailure) Read([]byte) (int, error) { return 0, r.err }

type readLastError struct {
	data []byte
	err  error
}

func (r *readLastError) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

type failingWriter struct {
	err     error
	partial bytes.Buffer
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, w.err
	}
	w.partial.Write(p[:1])
	return 1, w.err
}

type shortWriter struct{ partial bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.partial.Write(p[:1])
	return 1, nil
}

func memoryOptions(data []byte, mutate func(string, *inputFile)) (Options, *[]*inputFile) {
	var files []*inputFile
	return Options{OpenFile: func(path string) (File, error) {
		f := &inputFile{reader: bytes.NewReader(data)}
		if mutate != nil {
			mutate(path, f)
		}
		files = append(files, f)
		return f, nil
	}}, &files
}

func TestInputFailuresPreserveErrorsAndCloseOwnedFiles(t *testing.T) {
	token := errors.New("injected input failure")
	closeToken := errors.New("injected close failure")
	data := emptyArchive(t)
	for _, tc := range []struct {
		name      string
		args      []string
		mutate    func(string, *inputFile)
		want      error
		also      error
		wantFiles int
	}{
		{"before reader", nil, func(_ string, f *inputFile) { f.reader = readFailure{token} }, token, nil, 1},
		{"last read with data", nil, func(_ string, f *inputFile) { f.reader = &readLastError{data: data, err: token} }, token, nil, 1},
		{"after reader", nil, func(path string, f *inputFile) {
			if path == "after" {
				f.reader = readFailure{token}
			}
		}, token, nil, 2},
		{"stat", nil, func(_ string, f *inputFile) { f.statErr = token }, token, nil, 1},
		{"close", nil, func(_ string, f *inputFile) { f.closeErr = closeToken }, closeToken, nil, 1},
		{"read and close", nil, func(_ string, f *inputFile) { f.reader = readFailure{token}; f.closeErr = closeToken }, token, closeToken, 1},
		{"policy reader", []string{"--policy", "policy", "before", "after"}, func(_ string, f *inputFile) { f.reader = readFailure{token} }, token, nil, 1},
		{"policy close", []string{"--policy", "policy", "before", "after"}, func(_ string, f *inputFile) {
			f.reader = strings.NewReader(`{"schema_version":1,"required_paths":[]}`)
			f.closeErr = closeToken
		}, closeToken, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if args == nil {
				args = []string{"--format", "json", "before", "after"}
			}
			opts, files := memoryOptions(data, tc.mutate)
			var out bytes.Buffer
			code, err := run(context.Background(), args, &out, opts)
			if code != 2 || !errors.Is(err, tc.want) || (tc.also != nil && !errors.Is(err, tc.also)) {
				t.Fatalf("run = (%d, %v), want 2 preserving %v / %v", code, err, tc.want, tc.also)
			}
			if out.Len() != 0 {
				t.Fatalf("pre-render output: %q", out.String())
			}
			if len(*files) != tc.wantFiles {
				t.Fatalf("opened %d files, want %d", len(*files), tc.wantFiles)
			}
			for _, f := range *files {
				if f.closed != 1 {
					t.Fatalf("file closed %d times", f.closed)
				}
			}
			// Exercise the exported entry point with newly opened readers too.
			var diagnostics bytes.Buffer
			if code := Run(context.Background(), args, &out, &diagnostics, opts); code != 2 || !strings.Contains(diagnostics.String(), tc.want.Error()) {
				t.Fatalf("Run = %d, stderr %q", code, diagnostics.String())
			}
			if out.Len() != 0 {
				t.Fatalf("Run emitted failed report: %q", out.String())
			}
			for _, f := range *files {
				if f.closed != 1 {
					t.Fatalf("Run file closed %d times", f.closed)
				}
			}
		})
	}
}

func TestOpenFailureAndNonregularInput(t *testing.T) {
	failure := errors.New("cannot open injected file")
	opts := Options{OpenFile: func(string) (File, error) { return nil, failure }}
	var out, diagnostics bytes.Buffer
	code, err := run(context.Background(), []string{"before", "after"}, &out, opts)
	if code != 2 || !errors.Is(err, failure) || out.Len() != 0 {
		t.Fatalf("run = %d, %v, stdout %q", code, err, out.String())
	}
	if Run(context.Background(), []string{"before", "after"}, &out, &diagnostics, opts) != 2 || !strings.Contains(diagnostics.String(), failure.Error()) {
		t.Fatal("open failure lost")
	}
	for _, mode := range []fs.FileMode{fs.ModeDir, fs.ModeNamedPipe, fs.ModeSocket, fs.ModeDevice, fs.ModeSymlink} {
		opts, files := memoryOptions(nil, func(_ string, f *inputFile) { f.mode = mode })
		diagnostics.Reset()
		if code := Run(context.Background(), []string{"before", "after"}, &out, &diagnostics, opts); code != 2 || out.Len() != 0 || !strings.Contains(diagnostics.String(), "regular file") {
			t.Fatalf("mode %v: code %d, output %q / %q", mode, code, out.String(), diagnostics.String())
		}
		if len(*files) != 1 || (*files)[0].closed != 1 {
			t.Fatalf("mode %v: not closed", mode)
		}
	}
	for _, opts := range []Options{
		{OpenFile: func(string) (File, error) { return nil, nil }},
		{OpenFile: func(string) (File, error) { return &nilInfoFile{}, nil }},
	} {
		if code := Run(context.Background(), []string{"before", "after"}, &out, &diagnostics, opts); code != 2 || out.Len() != 0 {
			t.Fatalf("invalid opener result returned %d", code)
		}
	}
}

type nilInfoFile struct{}

func (*nilInfoFile) Stat() (fs.FileInfo, error) { return nil, nil }
func (*nilInfoFile) Read([]byte) (int, error)   { return 0, io.EOF }
func (*nilInfoFile) Close() error               { return nil }

func TestWriterFailuresAndShortWrites(t *testing.T) {
	failure := errors.New("injected writer failure")
	for _, args := range [][]string{{"--format", "json", "before", "after"}, {"--help"}, {"--version"}} {
		for _, want := range []error{failure, io.ErrShortWrite} {
			opts, files := memoryOptions(emptyArchive(t), nil)
			var writer io.Writer
			if want == failure {
				writer = &failingWriter{err: failure}
			} else {
				writer = &shortWriter{}
			}
			code, err := run(context.Background(), args, writer, opts)
			if code != 2 || !errors.Is(err, want) {
				t.Fatalf("%v: run = %d, %v, want %v", args, code, err, want)
			}
			for _, f := range *files {
				if f.closed != 1 {
					t.Fatal("writer failure leaked file")
				}
			}
			var diagnostics bytes.Buffer
			if code := Run(context.Background(), args, writer, &diagnostics, opts); code != 2 || !strings.Contains(diagnostics.String(), want.Error()) {
				t.Fatalf("Run = %d, diagnostics %q", code, diagnostics.String())
			}
		}
	}
	if code := Run(context.Background(), []string{"--bad"}, io.Discard, &shortWriter{}, Options{}); code != 2 {
		t.Fatal("diagnostic short write must fail")
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diagnostics bytes.Buffer
	calls := 0
	opts := Options{OpenFile: func(string) (File, error) { calls++; return nil, errors.New("must not open") }}
	code, err := run(ctx, []string{"before", "after"}, &out, opts)
	if code != 2 || !errors.Is(err, context.Canceled) || calls != 0 || out.Len() != 0 {
		t.Fatalf("canceled run: %d, %v, opens %d", code, err, calls)
	}
	if Run(ctx, []string{"before", "after"}, &out, &diagnostics, opts) != 2 || !strings.Contains(diagnostics.String(), context.Canceled.Error()) {
		t.Fatal("Run cancellation lost")
	}
	for _, policy := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		args := []string{"before", "after"}
		data := emptyArchive(t)
		if policy {
			args = append([]string{"--policy", "policy"}, args...)
			data = []byte(`{"schema_version":1,"required_paths":[]}`)
		}
		opts, files := memoryOptions(data, func(_ string, f *inputFile) { f.reader = cancelOnRead{cancel, f.reader} })
		code, err := run(ctx, args, &out, opts)
		cancel()
		if code != 2 || !errors.Is(err, context.Canceled) || out.Len() != 0 {
			t.Fatalf("policy=%v: %d, %v, stdout %q", policy, code, err, out.String())
		}
		if len(*files) != 1 || (*files)[0].closed != 1 {
			t.Fatal("canceled reader not closed")
		}
		// A fresh context proves cancellation through Run, without OS timing.
		ctx2, cancel2 := context.WithCancel(context.Background())
		opts2, _ := memoryOptions(data, func(_ string, f *inputFile) { f.reader = cancelOnRead{cancel2, f.reader} })
		diagnostics.Reset()
		if code := Run(ctx2, args, &out, &diagnostics, opts2); code != 2 || !strings.Contains(diagnostics.String(), context.Canceled.Error()) {
			t.Fatalf("Run canceled read = %d, %q", code, diagnostics.String())
		}
		cancel2()
	}
}

type cancelOnRead struct {
	cancel context.CancelFunc
	r      io.Reader
}

func (r cancelOnRead) Read(p []byte) (int, error) { r.cancel(); return r.r.Read(p) }

func TestResourcesClosedBeforeRendering(t *testing.T) {
	var events []string
	data := emptyArchive(t)
	opts := Options{OpenFile: func(path string) (File, error) {
		events = append(events, "open "+path)
		r := io.Reader(bytes.NewReader(data))
		if path == "policy" {
			r = strings.NewReader(`{"schema_version":1,"required_paths":[]}`)
		}
		return &eventFile{inputFile: inputFile{reader: r}, onClose: func() { events = append(events, "close "+path) }}, nil
	}}
	w := writeFunc(func(p []byte) (int, error) { events = append(events, "write"); return len(p), nil })
	if code := Run(context.Background(), []string{"--policy", "policy", "before", "after"}, w, io.Discard, opts); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := []string{"open policy", "close policy", "open before", "close before", "open after", "close after", "write"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events %v, want %v", events, want)
	}
}

type eventFile struct {
	inputFile
	onClose func()
}

func (f *eventFile) Close() error { f.onClose(); return f.inputFile.Close() }

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func TestUsageValidatesBeforeOpening(t *testing.T) {
	for _, args := range [][]string{
		nil, {"one"}, {"one", "two", "three"}, {"--bogus"}, {"--format", "yaml", "before", "after"},
		{"--ignore-path", "./bad", "before", "after"}, {"--ignore-prefix", "../bad", "before", "after"},
		{"--policy=", "before", "after"}, {"before", "--version"}, {"-", "after"}, {"https://host/input", "after"},
		{"--max-file-bytes", "0", "before", "after"}, {"--max-entries", "-1", "before", "after"},
	} {
		var out, diagnostics bytes.Buffer
		opts := Options{OpenFile: func(string) (File, error) { t.Fatalf("opened input for invalid args %v", args); return nil, nil }}
		if code := Run(context.Background(), args, &out, &diagnostics, opts); code != 2 || out.Len() != 0 || diagnostics.Len() == 0 {
			t.Fatalf("args %v: code %d, stdout %q stderr %q", args, code, out.String(), diagnostics.String())
		}
	}
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}} {
		var out, diagnostics bytes.Buffer
		opts := Options{Version: "test-build", OpenFile: func(string) (File, error) { t.Fatal("help/version opened a file"); return nil, nil }}
		if code := Run(context.Background(), args, &out, &diagnostics, opts); code != 0 || out.Len() == 0 || diagnostics.Len() != 0 {
			t.Fatalf("args %v: %d, %q / %q", args, code, out.String(), diagnostics.String())
		}
		if args[0] == "--version" && out.String() != "rootfs-diff test-build\n" {
			t.Fatalf("injected version %q", out.String())
		}
	}
}

func TestRunNilInputs(t *testing.T) {
	var diagnostics bytes.Buffer
	if Run(nil, []string{"before", "after"}, io.Discard, &diagnostics, Options{}) != 2 || diagnostics.Len() == 0 {
		t.Fatal("nil context must fail")
	}
	if Run(context.Background(), []string{"--help"}, nil, &diagnostics, Options{}) != 2 {
		t.Fatal("nil stdout must fail")
	}
	if Run(context.Background(), []string{"--help"}, io.Discard, nil, Options{}) != 2 {
		t.Fatal("nil stderr must fail")
	}
}
