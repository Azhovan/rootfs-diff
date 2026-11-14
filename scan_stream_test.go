package rootfsdiff_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	rootfsdiff "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/internal/testfixture"
)

func streamFixture(t testing.TB) []byte {
	t.Helper()
	data, err := testfixture.Tar(testfixture.Entry{
		Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Size: 1024, Mode: 0640},
		Data:   bytes.Repeat([]byte{'x'}, 1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requireStreamFailure(t *testing.T, m rootfsdiff.Manifest, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v; want errors.Is(_, %v)", err, want)
	}
	if !reflect.DeepEqual(m, rootfsdiff.Manifest{}) {
		t.Fatalf("failure returned partial manifest: %#v", m)
	}
}

// errorAtReader injects an error on the same call as the last bytes ending at
// offset, then continues normally. This exercises errors that io.ReadFull can
// hide when the returned bytes completely fill its requested header block.
type errorAtReader struct {
	data   []byte
	offset int
	read   int
	err    error
	fired  bool
}

func (r *errorAtReader) Read(p []byte) (int, error) {
	if !r.fired && r.read == r.offset {
		r.fired = true
		return 0, r.err
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	if len(p) > 512 {
		p = p[:512]
	}
	if !r.fired && len(p) > r.offset-r.read {
		p = p[:r.offset-r.read]
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	r.read += n
	if !r.fired && r.read == r.offset {
		r.fired = true
		return n, r.err
	}
	return n, nil
}

func TestScanPreservesUnderlyingReadErrors(t *testing.T) {
	data := streamFixture(t)
	for _, test := range []struct {
		name   string
		offset int
	}{
		{"before input", 0},
		{"partial header", 100},
		{"complete header", 512},
		{"payload boundary", 1536},
		{"first zero terminator", 2048},
		{"second zero terminator", 2560},
	} {
		t.Run(test.name, func(t *testing.T) {
			sentinel := errors.New("test input failure")
			r := &errorAtReader{data: data, offset: test.offset, err: sentinel}
			m, err := rootfsdiff.Scan(context.Background(), r, rootfsdiff.ScanOptions{})
			requireStreamFailure(t, m, err, sentinel)
		})
	}
}

type cancelReader struct {
	r      io.Reader
	cancel context.CancelFunc
	reads  int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	r.reads++
	n, err := r.r.Read(p)
	r.cancel()
	return n, err
}

func TestScanCancellationAtReadBoundary(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := &cancelReader{r: bytes.NewReader(streamFixture(t)), cancel: cancel}
		m, err := rootfsdiff.Scan(ctx, r, rootfsdiff.ScanOptions{})
		requireStreamFailure(t, m, err, context.Canceled)
		if r.reads != 0 {
			t.Fatalf("canceled scan read input %d times", r.reads)
		}
	})
	t.Run("cancel after read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := &cancelReader{r: bytes.NewReader(streamFixture(t)), cancel: cancel}
		m, err := rootfsdiff.Scan(ctx, r, rootfsdiff.ScanOptions{})
		requireStreamFailure(t, m, err, context.Canceled)
		if r.reads != 1 {
			t.Fatalf("scan made %d underlying reads after cancellation; want one", r.reads)
		}
	})
}

type noProgressReader struct{ reads int }

func (r *noProgressReader) Read([]byte) (int, error) {
	r.reads++
	return 0, nil
}

func TestScanRejectsPersistentNoProgress(t *testing.T) {
	r := new(noProgressReader)
	m, err := rootfsdiff.Scan(context.Background(), r, rootfsdiff.ScanOptions{})
	requireStreamFailure(t, m, err, io.ErrNoProgress)
	if r.reads > 1000 {
		t.Fatalf("no-progress input consumed %d reads", r.reads)
	}
}

type ownedReader struct {
	io.Reader
	closed bool
}

func (r *ownedReader) Close() error {
	r.closed = true
	return errors.New("caller-owned Close must not be called")
}

func TestScanDoesNotCloseInput(t *testing.T) {
	raw := streamFixture(t)
	compressed, err := testfixture.Gzip(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{raw, compressed, raw[:100]} {
		r := &ownedReader{Reader: bytes.NewReader(data)}
		_, _ = rootfsdiff.Scan(context.Background(), r, rootfsdiff.ScanOptions{})
		if r.closed {
			t.Fatal("Scan closed the caller's reader")
		}
	}
}

func TestScanDoesNotAccessSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "must-not-exist")
	data, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{
		Name: "link", Typeflag: tar.TypeSymlink, Linkname: target, Mode: 0777,
	}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := rootfsdiff.Scan(context.Background(), bytes.NewReader(data), rootfsdiff.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 || m.Entries[0].LinkTarget != target {
		t.Fatalf("symlink manifest = %#v", m)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target state = %v; want absent", err)
	}
	items, err := os.ReadDir(dir)
	if err != nil || len(items) != 0 {
		t.Fatalf("host directory changed: %v, %v", items, err)
	}
}

// zeroStream is a generated source; its storage is independent of payload size.
type zeroStream struct{}

func (zeroStream) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type requestReader struct {
	io.Reader
	maximum int
}

func (r *requestReader) Read(p []byte) (int, error) {
	if len(p) > r.maximum {
		r.maximum = len(p)
	}
	return r.Reader.Read(p)
}

func generatedTar(t *testing.T, size int64) *requestReader {
	t.Helper()
	var header bytes.Buffer
	w := tar.NewWriter(&header)
	if err := w.WriteHeader(&tar.Header{Name: "payload", Typeflag: tar.TypeReg, Size: size, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	// Do not close w: its payload comes from the generated stream below.
	padding := (512 - size%512) % 512
	return &requestReader{Reader: io.MultiReader(
		bytes.NewReader(header.Bytes()),
		io.LimitReader(zeroStream{}, size),
		io.LimitReader(zeroStream{}, padding+1024),
	)}
}

func TestScanPayloadAllocationBound(t *testing.T) {
	// Independent digests computed with Python hashlib over repeated zero blocks.
	cases := []struct {
		size int64
		sum  string
	}{
		{1 << 20, "30e14955ebf1352266dc2ff8067e68104607e750abb9d3b36582b8af909fcb58"},
		{128 << 20, "254bcc3fc4f27172636df4bf32de9f107f620d559b20d760197e452b97453917"},
	}
	// Warm up crypto/runtime initialization before measuring both equal-entry
	// scans. There is no allocation or elapsed-time ratio requirement.
	if _, err := rootfsdiff.Scan(context.Background(), generatedTar(t, 0), rootfsdiff.ScanOptions{}); err != nil {
		t.Fatal(err)
	}
	var allocated [2]uint64
	for i, test := range cases {
		r := generatedTar(t, test.size)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		m, err := rootfsdiff.Scan(context.Background(), r, rootfsdiff.ScanOptions{})
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatal(err)
		}
		want := rootfsdiff.Entry{Path: "payload", Kind: rootfsdiff.KindRegular, Mode: 0600, Size: test.size, SHA256: test.sum}
		if len(m.Entries) != 1 || m.Entries[0] != want {
			t.Fatalf("generated payload manifest = %#v; want %#v", m, want)
		}
		allocated[i] = after.TotalAlloc - before.TotalAlloc
		t.Logf("payload=%d bytes entries=1 allocated=%d bytes allocations=%d largest_input_read=%d bytes", test.size, allocated[i], after.Mallocs-before.Mallocs, r.maximum)
		if r.maximum > 64<<10 {
			t.Fatalf("input read requested %d bytes; want bounded chunks", r.maximum)
		}
		// A broad absolute allowance permits runtime noise and instrumentation,
		// while retaining a 128 MiB payload would exceed it decisively.
		if allocated[i] > 8<<20 {
			t.Fatalf("scan allocated %d bytes for one entry", allocated[i])
		}
	}
	if allocated[1] > allocated[0]+4<<20 {
		t.Fatalf("allocation grew with payload: 1 MiB=%d, 128 MiB=%d", allocated[0], allocated[1])
	}
}

func FuzzScan(f *testing.F) {
	valid := streamFixture(f)
	compressed, err := testfixture.Gzip(valid)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{nil, make([]byte, 1024), valid, compressed, valid[:511], {0x1f, 0x8b, 0, 0}} {
		f.Add(seed)
	}
	pax, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{
		Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"VENDOR.note": "opaque"},
	}})
	if err != nil {
		f.Fatal(err)
	}
	gnu, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{
		Name: string(bytes.Repeat([]byte{'n'}, 160)), Typeflag: tar.TypeReg, Format: tar.FormatGNU,
	}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(pax)
	f.Add(gnu)
	f.Add(append(append([]byte(nil), pax[:1024]...), pax...))
	badPAX, err := testfixture.Tar(testfixture.Entry{Header: tar.Header{
		Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"vvv": "not-a-number", "VENDOR.hidden": "opaque"},
	}})
	if err != nil {
		f.Fatal(err)
	}
	badPAX = bytes.ReplaceAll(badPAX, []byte("vvv="), []byte("uid="))
	f.Add(append(append([]byte(nil), badPAX[:1024]...), pax...))
	opts := rootfsdiff.ScanOptions{MaxEntries: 8, MaxArchiveBytes: 128 << 10, MaxFileBytes: 16 << 10, MaxPathBytes: 256}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 64<<10 {
			t.Skip()
		}
		m, err := rootfsdiff.Scan(context.Background(), bytes.NewReader(input), opts)
		if err != nil {
			if !reflect.DeepEqual(m, rootfsdiff.Manifest{}) {
				t.Fatalf("failed scan returned partial data: %#v", m)
			}
			return
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("successful scan returned invalid manifest: %v", err)
		}
		if len(m.Entries) > 8 {
			t.Fatalf("entry budget exceeded: %d", len(m.Entries))
		}
		for i, e := range m.Entries {
			if e.Size > 16<<10 || len(e.Path) > 256 || len(e.LinkTarget) > 256 || (i > 0 && m.Entries[i-1].Path >= e.Path) {
				t.Fatalf("successful scan exceeded a limit or was unsorted: %#v", m)
			}
		}
		second, err := rootfsdiff.Scan(context.Background(), bytes.NewReader(input), opts)
		if err != nil || !reflect.DeepEqual(m, second) {
			t.Fatalf("scan is nondeterministic: first=%#v second=%#v error=%v", m, second, err)
		}
	})
}
