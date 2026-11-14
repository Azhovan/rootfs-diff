package testfixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTarHeaderVariants(t *testing.T) {
	for _, format := range []tar.Format{tar.FormatUSTAR, tar.FormatGNU, tar.FormatPAX} {
		t.Run(format.String(), func(t *testing.T) {
			name := "etc/device.conf"
			if format != tar.FormatUSTAR {
				name = "etc/" + strings.Repeat("x", 150)
			}
			entries := []Entry{{Header: tar.Header{
				Name: name, Typeflag: tar.TypeReg, Format: format,
				Mode: 06751, Uid: 12345, Gid: 23456, Size: 4,
				Uname: "recorded-user", Gname: "recorded-group",
				ModTime: time.Unix(1234567890, 0),
			}, Data: []byte("a\x00b\n")}}
			if format == tar.FormatPAX {
				entries[0].Header.PAXRecords = map[string]string{"VENDOR.note": "uncompared"}
				entries[0].Header.Xattrs = map[string]string{"security.capability": "recorded-value"}
			}
			before := entries[0].Header
			before.PAXRecords = mapCopy(before.PAXRecords)
			before.Xattrs = mapCopy(before.Xattrs)
			data, err := Tar(entries...)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(entries[0].Header, before) {
				t.Fatal("Tar mutated its input header")
			}
			r := tar.NewReader(bytes.NewReader(data))
			h, err := r.Next()
			if err != nil {
				t.Fatal(err)
			}
			if h.Name != name || h.Typeflag != tar.TypeReg || h.Format != format || h.Mode != 06751 || h.Uid != 12345 || h.Gid != 23456 || h.Size != 4 || h.Uname != "recorded-user" || h.Gname != "recorded-group" || !h.ModTime.Equal(time.Unix(1234567890, 0)) {
				t.Fatalf("unexpected header: %+v", h)
			}
			if format == tar.FormatPAX && h.PAXRecords["VENDOR.note"] != "uncompared" {
				t.Fatalf("missing PAX metadata: %+v", h.PAXRecords)
			}
			if format == tar.FormatPAX && h.Xattrs["security.capability"] != "recorded-value" {
				t.Fatalf("missing xattr metadata: %+v", h.Xattrs)
			}
			payload, err := io.ReadAll(r)
			if err != nil || !bytes.Equal(payload, []byte{'a', 0, 'b', '\n'}) {
				t.Fatalf("payload = %q, %v", payload, err)
			}
			if _, err := r.Next(); !errors.Is(err, io.EOF) {
				t.Fatalf("after payload: %v", err)
			}
		})
	}
}

func TestTarEntryKinds(t *testing.T) {
	entries := []Entry{
		{Header: tar.Header{Name: "dev", Typeflag: tar.TypeDir, Mode: 02755, Uid: 10, Gid: 20}},
		{Header: tar.Header{Name: "symlink", Typeflag: tar.TypeSymlink, Linkname: "/etc/../target", Mode: 0777}},
		{Header: tar.Header{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "target", Mode: 0600}},
		{Header: tar.Header{Name: "dev/console", Typeflag: tar.TypeChar, Devmajor: 5, Devminor: 1, Mode: 0600}},
		{Header: tar.Header{Name: "dev/disk", Typeflag: tar.TypeBlock, Devmajor: 8, Devminor: 3, Mode: 0660}},
		{Header: tar.Header{Name: "pipe", Typeflag: tar.TypeFifo, Mode: 01644}},
		{Header: tar.Header{Name: "target", Typeflag: tar.TypeReg, Mode: 0644}},
	}
	data, err := Tar(entries...)
	if err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(bytes.NewReader(data))
	// These expectations are independent of the builder's entry slice.
	want := []struct {
		name     string
		kind     byte
		mode     int64
		uid, gid int
		link     string
		major    int64
		minor    int64
	}{
		{"dev", tar.TypeDir, 02755, 10, 20, "", 0, 0},
		{"symlink", tar.TypeSymlink, 0777, 0, 0, "/etc/../target", 0, 0},
		{"hardlink", tar.TypeLink, 0600, 0, 0, "target", 0, 0},
		{"dev/console", tar.TypeChar, 0600, 0, 0, "", 5, 1},
		{"dev/disk", tar.TypeBlock, 0660, 0, 0, "", 8, 3},
		{"pipe", tar.TypeFifo, 01644, 0, 0, "", 0, 0},
		{"target", tar.TypeReg, 0644, 0, 0, "", 0, 0},
	}
	for _, item := range want {
		h, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if h.Name != item.name || h.Typeflag != item.kind || h.Mode != item.mode || h.Uid != item.uid || h.Gid != item.gid || h.Linkname != item.link || h.Devmajor != item.major || h.Devminor != item.minor || h.Size != 0 {
			t.Fatalf("unexpected header: %+v", h)
		}
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after entries: %v", err)
	}
}

func TestTarAndGzipDeterministic(t *testing.T) {
	entry := Entry{Header: tar.Header{
		Name: "etc/config", Typeflag: tar.TypeReg, Mode: 0644, Size: 5, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"VENDOR.z": "last", "VENDOR.a": "first"},
	}, Data: []byte("hello")}
	first, err := Tar(entry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Tar(entry)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("tar bytes changed: %v", err)
	}
	one, err := Gzip(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Gzip(second)
	if err != nil || !bytes.Equal(one, two) {
		t.Fatalf("gzip bytes changed: %v", err)
	}
	r, err := gzip.NewReader(bytes.NewReader(one))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "" || r.Comment != "" || !r.ModTime.IsZero() || r.OS != 255 {
		t.Fatalf("unexpected gzip header: %+v", r.Header)
	}
	decoded, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(decoded, first) {
		t.Fatalf("gzip decoded data differs: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCorruption(t *testing.T) {
	data, err := Tar(Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Size: 5}, Data: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := Patch(data, 0, []byte{'!'})
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != 'f' || broken[0] != '!' {
		t.Fatal("Patch did not preserve input or replace requested byte")
	}
	if _, err := tar.NewReader(bytes.NewReader(broken)).Next(); !errors.Is(err, tar.ErrHeader) {
		t.Fatalf("corrupt header = %v", err)
	}
	r := tar.NewReader(bytes.NewReader(data[:514]))
	if _, err := r.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated payload = %v", err)
	}
	gz, err := Gzip(data)
	if err != nil {
		t.Fatal(err)
	}
	gz, err = Patch(gz, len(gz)-8, []byte{gz[len(gz)-8] ^ 1})
	if err != nil {
		t.Fatal(err)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	if _, err := io.ReadAll(gzipReader); !errors.Is(err, gzip.ErrChecksum) {
		t.Fatalf("corrupt gzip trailer = %v", err)
	}
}

func TestBuilderErrors(t *testing.T) {
	for _, entry := range []Entry{
		{Header: tar.Header{Name: "short", Size: 5}, Data: []byte("hi")},
		{Header: tar.Header{Name: "long", Size: 1}, Data: []byte("hi")},
		{Header: tar.Header{Name: "negative", Size: -1}},
		{Header: tar.Header{Name: "dir", Typeflag: tar.TypeDir}, Data: []byte("payload")},
	} {
		if _, err := Tar(entry); err == nil {
			t.Fatalf("accepted invalid entry %q", entry.Header.Name)
		}
	}
	for _, offset := range []int{-1, 3, int(^uint(0) >> 1)} {
		if _, err := Patch([]byte{1, 2}, offset, []byte{3}); err == nil {
			t.Fatalf("accepted patch offset %d", offset)
		}
	}
	if _, err := Patch([]byte{1, 2}, 1, []byte{3, 4}); err == nil {
		t.Fatal("accepted overflowing replacement")
	}
	empty, err := Tar()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tar.NewReader(bytes.NewReader(empty)).Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("empty archive = %v", err)
	}
}

func mapCopy(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
