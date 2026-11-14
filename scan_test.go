package rootfsdiff_test

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	diff "github.com/Azhovan/rootfs-diff"
	"github.com/Azhovan/rootfs-diff/internal/testfixture"
)

const emptySHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
const helloSHA = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func scanArchive(t *testing.T, entries ...testfixture.Entry) []byte {
	t.Helper()
	b, err := testfixture.Tar(entries...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func scanGzip(t *testing.T, b []byte) []byte {
	t.Helper()
	out, err := testfixture.Gzip(b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func scanOK(t *testing.T, b []byte, opts diff.ScanOptions) diff.Manifest {
	t.Helper()
	m, err := diff.Scan(context.Background(), bytes.NewReader(b), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("invalid successful manifest: %v", err)
	}
	return m
}

func scanFails(t *testing.T, b []byte, opts diff.ScanOptions, category error) error {
	t.Helper()
	m, err := diff.Scan(context.Background(), bytes.NewReader(b), opts)
	if err == nil || (category != nil && !errors.Is(err, category)) {
		t.Fatalf("Scan error = %v, want %v", err, category)
	}
	if len(m.Entries) != 0 || len(m.Warnings) != 0 {
		t.Fatal("failure returned partial manifest")
	}
	return err
}

func scanEntry(name string, kind byte, data string) testfixture.Entry {
	return testfixture.Entry{Header: tar.Header{Name: name, Typeflag: kind, Size: int64(len(data)), Mode: 0644}, Data: []byte(data)}
}

func TestScanSupportedEntries(t *testing.T) {
	entries := []testfixture.Entry{
		{Header: tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0777}},
		{Header: tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 01775, Uid: 222, Gid: 333}},
		{Header: tar.Header{Name: "implicit/file", Typeflag: tar.TypeReg, Size: 5, Mode: 06751, Uid: 1234567, Gid: 2345678, Uname: "host-independent", Gname: "ignored"}, Data: []byte("hello")},
		scanEntry("empty", tar.TypeReg, ""),
		{Header: tar.Header{Name: "symlink", Typeflag: tar.TypeSymlink, Linkname: "/../device/../target", Mode: 0777, Uid: 12, Gid: 34}},
		{Header: tar.Header{Name: "char", Typeflag: tar.TypeChar, Devmajor: 4, Devminor: 65, Mode: 0600, Uid: 1, Gid: 2}},
		{Header: tar.Header{Name: "block", Typeflag: tar.TypeBlock, Devmajor: 8, Devminor: 2, Mode: 0660}},
		{Header: tar.Header{Name: "fifo", Typeflag: tar.TypeFifo, Mode: 0620}},
	}
	want := []diff.Entry{
		{Path: "block", Kind: diff.KindBlockDevice, Mode: 0660, DeviceMajor: 8, DeviceMinor: 2},
		{Path: "char", Kind: diff.KindCharDevice, Mode: 0600, UID: 1, GID: 2, DeviceMajor: 4, DeviceMinor: 65},
		{Path: "dir", Kind: diff.KindDirectory, Mode: 01775, UID: 222, GID: 333},
		{Path: "empty", Kind: diff.KindRegular, Mode: 0644, SHA256: emptySHA},
		{Path: "fifo", Kind: diff.KindFIFO, Mode: 0620},
		{Path: "implicit/file", Kind: diff.KindRegular, Mode: 06751, UID: 1234567, GID: 2345678, Size: 5, SHA256: helloSHA},
		{Path: "symlink", Kind: diff.KindSymlink, Mode: 0777, UID: 12, GID: 34, LinkTarget: "/../device/../target"},
	}
	data := scanArchive(t, entries...)
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprint(compressed), func(t *testing.T) {
			input := data
			if compressed {
				input = scanGzip(t, input)
			}
			m := scanOK(t, input, diff.ScanOptions{})
			if !reflect.DeepEqual(m.Entries, want) || len(m.Warnings) != 0 {
				t.Fatalf("manifest = %#v, want %#v", m, want)
			}
		})
	}
	// Changing header-only directory size metadata does not create payload.
	data = scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "dir", Typeflag: tar.TypeDir, Size: 4096}})
	if m := scanOK(t, data, diff.ScanOptions{}); m.Entries[0].Size != 0 {
		t.Fatal("directory size was retained")
	}
}

func TestScanFormatsAndNormalization(t *testing.T) {
	for _, format := range []tar.Format{tar.FormatUSTAR, tar.FormatGNU, tar.FormatPAX} {
		t.Run(format.String(), func(t *testing.T) {
			long := strings.Repeat("segment/", 18) + "file"
			data := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: long, Typeflag: tar.TypeReg, Format: format}})
			if m := scanOK(t, data, diff.ScanOptions{}); len(m.Entries) != 1 || m.Entries[0].Path != long || m.Entries[0].SHA256 != emptySHA {
				t.Fatalf("long-name manifest: %#v", m)
			}
		})
	}
	for _, name := range []string{"./a//./b", "a/b", "a///b", "a/./b"} {
		data := scanArchive(t, scanEntry(name, tar.TypeReg, ""))
		if m := scanOK(t, data, diff.ScanOptions{}); m.Entries[0].Path != "a/b" {
			t.Fatalf("canonical name %q = %q", name, m.Entries[0].Path)
		}
	}
	for _, name := range []string{`a\..\b`, "café/配置", "file\nwith\tcontrols"} {
		data := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: name, Typeflag: tar.TypeReg, Format: tar.FormatGNU}})
		if m := scanOK(t, data, diff.ScanOptions{}); m.Entries[0].Path != name {
			t.Fatalf("literal name %q = %q", name, m.Entries[0].Path)
		}
	}
	for _, target := range []string{"../outside", "/absolute/device", "café/../配置", ""} {
		data := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: target}})
		if got := scanOK(t, data, diff.ScanOptions{}).Entries[0].LinkTarget; got != target {
			t.Fatalf("target %q = %q", target, got)
		}
	}
}

func TestScanInvalidPathsAndAncestors(t *testing.T) {
	for _, name := range []string{"/absolute", "../file", "a/../b", "a/..", "a/../../b", "bad\xff"} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			data := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: name, Typeflag: tar.TypeReg, Format: tar.FormatGNU}})
			scanFails(t, data, diff.ScanOptions{}, diff.ErrInvalidInput)
		})
	}
	for _, name := range []string{".", "./", "././"} {
		input := scanArchive(t, scanEntry("placeholder", tar.TypeReg, ""))
		input = patchTarHeader(t, input, func(h []byte) { clear(h[:100]); copy(h[:100], name) })
		scanFails(t, input, diff.ScanOptions{}, diff.ErrInvalidInput)
	}
	for _, pair := range [][2]string{{"a/b", "./a//b"}, {"d", "./d"}} {
		scanFails(t, scanArchive(t, scanEntry(pair[0], tar.TypeReg, ""), scanEntry(pair[1], tar.TypeReg, "")), diff.ScanOptions{}, diff.ErrInvalidInput)
	}
	for _, kind := range []byte{tar.TypeReg, tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		parent := testfixture.Entry{Header: tar.Header{Name: "a", Typeflag: kind, Linkname: "target"}}
		if kind != tar.TypeSymlink && kind != tar.TypeLink {
			parent.Header.Linkname = ""
		}
		for _, reverse := range []bool{false, true} {
			entries := []testfixture.Entry{parent, scanEntry("a/b", tar.TypeReg, ""), scanEntry("target", tar.TypeReg, "")}
			if reverse {
				entries[0], entries[1] = entries[1], entries[0]
			}
			scanFails(t, scanArchive(t, entries...), diff.ScanOptions{}, diff.ErrInvalidInput)
		}
	}
	scanFails(t, scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "bad\xff", Format: tar.FormatGNU}}), diff.ScanOptions{}, diff.ErrInvalidInput)
}

func TestScanHardlinks(t *testing.T) {
	entries := []testfixture.Entry{
		{Header: tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: "./b", Mode: 0000, Uid: 999, Gid: 999}},
		{Header: tar.Header{Name: "b", Typeflag: tar.TypeLink, Linkname: "./dir//terminal", Mode: 0777}},
		{Header: tar.Header{Name: "dir/terminal", Typeflag: tar.TypeReg, Size: 5, Mode: 04750, Uid: 123, Gid: 456}, Data: []byte("hello")},
	}
	want := []diff.Entry{
		{Path: "a", Kind: diff.KindHardlink, LinkTarget: "b", Mode: 04750, UID: 123, GID: 456, Size: 5, SHA256: helloSHA},
		{Path: "b", Kind: diff.KindHardlink, LinkTarget: "dir/terminal", Mode: 04750, UID: 123, GID: 456, Size: 5, SHA256: helloSHA},
		{Path: "dir/terminal", Kind: diff.KindRegular, Mode: 04750, UID: 123, GID: 456, Size: 5, SHA256: helloSHA},
	}
	if got := scanOK(t, scanArchive(t, entries...), diff.ScanOptions{}); !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("hardlinks = %#v", got)
	}
	for _, target := range []string{"missing", "a", "/absolute", "../parent", ".", "bad\xff"} {
		scanFails(t, scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: target, Format: tar.FormatGNU}}), diff.ScanOptions{}, diff.ErrInvalidInput)
	}
	scanFails(t, scanArchive(t,
		testfixture.Entry{Header: tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: "b"}},
		testfixture.Entry{Header: tar.Header{Name: "b", Typeflag: tar.TypeLink, Linkname: "a"}},
	), diff.ScanOptions{}, diff.ErrInvalidInput)
	for _, kind := range []byte{tar.TypeDir, tar.TypeSymlink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		scanFails(t, scanArchive(t,
			testfixture.Entry{Header: tar.Header{Name: "a", Typeflag: tar.TypeLink, Linkname: "b"}},
			testfixture.Entry{Header: tar.Header{Name: "b", Typeflag: kind}},
		), diff.ScanOptions{}, diff.ErrInvalidInput)
	}
}

func TestScanLimits(t *testing.T) {
	data := scanArchive(t, scanEntry("abc", tar.TypeReg, "hello"))
	for _, opts := range []diff.ScanOptions{{MaxFileBytes: 5}, {MaxEntries: 1}, {MaxArchiveBytes: int64(len(data))}, {MaxPathBytes: 3}} {
		scanOK(t, data, opts)
	}
	for _, tc := range []struct {
		name string
		opts diff.ScanOptions
	}{
		{"max_file_bytes", diff.ScanOptions{MaxFileBytes: 4}},
		{"max_archive_bytes", diff.ScanOptions{MaxArchiveBytes: int64(len(data) - 1)}},
		{"max_path_bytes", diff.ScanOptions{MaxPathBytes: 2}},
	} {
		err := scanFails(t, data, tc.opts, diff.ErrLimitExceeded)
		var limit *diff.LimitError
		if !errors.As(err, &limit) || limit.Limit != tc.name || limit.Actual <= limit.Maximum {
			t.Fatalf("typed limit error = %#v", limit)
		}
	}
	scanFails(t, scanArchive(t, scanEntry("a", tar.TypeReg, ""), scanEntry("b", tar.TypeReg, "")), diff.ScanOptions{MaxEntries: 1}, diff.ErrLimitExceeded)
	scanFails(t, scanArchive(t, testfixture.Entry{Header: tar.Header{Name: ".", Typeflag: tar.TypeDir}}, scanEntry("b", tar.TypeReg, "")), diff.ScanOptions{MaxEntries: 1}, diff.ErrLimitExceeded)
	for _, opts := range []diff.ScanOptions{{MaxEntries: -1}, {MaxArchiveBytes: -1}, {MaxFileBytes: -1}, {MaxPathBytes: -1}} {
		scanFails(t, data, opts, diff.ErrInvalidInput)
	}
	// Count decoded metadata extensions, padding, and every gzip member.
	pax := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: strings.Repeat("a", 160), Typeflag: tar.TypeReg, Format: tar.FormatPAX}})
	scanFails(t, pax, diff.ScanOptions{MaxArchiveBytes: 700}, diff.ErrLimitExceeded)
	scanFails(t, scanGzip(t, data), diff.ScanOptions{MaxArchiveBytes: int64(len(data) - 1)}, diff.ErrLimitExceeded)
	scanFails(t, append(scanGzip(t, data), scanGzip(t, make([]byte, 512))...), diff.ScanOptions{MaxArchiveBytes: int64(len(data))}, diff.ErrLimitExceeded)
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink} {
		target := "abc"
		input := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "x", Typeflag: kind, Linkname: target}}, scanEntry("abc", tar.TypeReg, ""))
		scanOK(t, input, diff.ScanOptions{MaxPathBytes: 3})
		scanFails(t, input, diff.ScanOptions{MaxPathBytes: 2}, diff.ErrLimitExceeded)
	}
	// Limit the fully decoded long name, not the short physical PAX header.
	scanFails(t, pax, diff.ScanOptions{MaxPathBytes: 159}, diff.ErrLimitExceeded)
	scanOK(t, pax, diff.ScanOptions{MaxPathBytes: 160})
}

// patchTarHeader alters only a physical header and computes its tar checksum.
// It is a fixture encoder, independent of scanner normalization/validation.
func patchTarHeader(t *testing.T, archive []byte, edit func([]byte)) []byte {
	t.Helper()
	out := append([]byte(nil), archive...)
	edit(out[:512])
	for i := 148; i < 156; i++ {
		out[i] = ' '
	}
	var sum int
	for _, b := range out[:512] {
		sum += int(b)
	}
	copy(out[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return out
}

func TestScanUnsupported(t *testing.T) {
	base := scanArchive(t, scanEntry("file", tar.TypeReg, ""))
	for _, flag := range []byte{'Q', tar.TypeCont, tar.TypeGNUSparse} {
		input := patchTarHeader(t, base, func(h []byte) { h[156] = flag })
		if flag == tar.TypeGNUSparse {
			// GNU sparse header with no extents and zero logical size.
			input = patchTarHeader(t, input, func(h []byte) { copy(h[257:265], "ustar  \x00") })
		}
		scanFails(t, input, diff.ScanOptions{}, diff.ErrUnsupportedInput)
	}
	global := scanArchive(t, testfixture.Entry{Header: tar.Header{Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "global"}}})
	scanFails(t, global, diff.ScanOptions{}, diff.ErrUnsupportedInput)
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		input := patchTarHeader(t, base, func(h []byte) { h[156] = kind; copy(h[124:136], "00000000001\x00") })
		scanFails(t, input, diff.ScanOptions{}, diff.ErrUnsupportedInput)
	}
	for _, key := range []string{"GNU.sparse.future", "GNU.sparse.major", "SCHILY.sparse.map", "SCHILY.realsize", "SCHILY.filetype", "SUN.holesdata"} {
		// tar.Writer filters GNU sparse records, so encode a same-length key
		// substitution in an otherwise valid PAX extension fixture.
		placeholder := strings.Repeat("v", len(key))
		value := ""
		if key == "SCHILY.filetype" {
			value = "sparse"
		}
		input := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{placeholder: value}}})
		input = bytes.ReplaceAll(input, []byte(placeholder+"="), []byte(key+"="))
		scanFails(t, input, diff.ScanOptions{}, diff.ErrUnsupportedInput)
	}
}

func TestScanMetadataWarnings(t *testing.T) {
	pax := map[string]string{"VENDOR.unknown": "SECRET-DO-NOT-LEAK", "SCHILY.acl.access": "SECRET-ACL", "SCHILY.xattr.security.capability": "SECRET-CAP", "SCHILY.xattr.security.selinux": "SECRET-SELINUX", "SCHILY.xattr.user.note": "SECRET-XATTR"}
	input := scanArchive(t,
		testfixture.Entry{Header: tar.Header{Name: ".", Typeflag: tar.TypeDir, Format: tar.FormatPAX, PAXRecords: map[string]string{"VENDOR.root": "SECRET-ROOT"}}},
		testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: pax, ModTime: time.Unix(10, 123), AccessTime: time.Unix(20, 456), ChangeTime: time.Unix(30, 789), Uname: "ignored", Gname: "ignored"}},
	)
	want := []diff.Warning{
		{Path: ".", Code: diff.WarningUncomparedMetadata, Key: "VENDOR.root"},
		{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "SCHILY.acl.access"},
		{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "SCHILY.xattr.security.capability"},
		{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "SCHILY.xattr.security.selinux"},
		{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "SCHILY.xattr.user.note"},
		{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "VENDOR.unknown"},
	}
	m := scanOK(t, input, diff.ScanOptions{})
	if !reflect.DeepEqual(m.Warnings, want) || strings.Contains(fmt.Sprint(m), "SECRET") {
		t.Fatalf("warnings = %#v", m.Warnings)
	}
	bad := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{"VENDOR.bad": "value"}}})
	bad = bytes.ReplaceAll(bad, []byte("VENDOR.bad="), []byte("VENDOR.ba\xff="))
	scanFails(t, bad, diff.ScanOptions{}, diff.ErrInvalidInput)
	// Neither name fields nor time fields affect entries or produce warnings.
	one := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, Uname: "first", Gname: "first", ModTime: time.Unix(1, 2)}})
	two := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, Uname: "other", Gname: "other", ModTime: time.Unix(9, 8)}})
	if !reflect.DeepEqual(scanOK(t, one, diff.ScanOptions{}), scanOK(t, two, diff.ScanOptions{})) {
		t.Fatal("deliberately ignored metadata changed the manifest")
	}
}

func TestScanEOFAndGzipValidation(t *testing.T) {
	full := scanArchive(t, scanEntry("file", tar.TypeReg, "hello"))
	completeEntry := full[:1024]
	for name, data := range map[string][]byte{
		"empty stream":       nil,
		"empty archive":      make([]byte, 1024),
		"no terminators":     completeEntry,
		"EOF during padding": full[:1000],
		"EOF after payload":  full[:517],
		"one terminator":     full[:1536],
		"two terminators":    full,
		"zero padding":       append(append([]byte(nil), full...), make([]byte, 317)...),
	} {
		t.Run(name, func(t *testing.T) {
			plain := scanOK(t, data, diff.ScanOptions{})
			compressed := scanOK(t, scanGzip(t, data), diff.ScanOptions{})
			if !reflect.DeepEqual(plain, compressed) {
				t.Fatal("compression affected manifest")
			}
		})
	}
	for name, data := range map[string][]byte{
		"partial header":     full[:511],
		"partial payload":    full[:514],
		"partial terminator": full[:1200],
		"checksum":           append([]byte{full[0] ^ 1}, full[1:]...),
		"nonzero trailing":   append(append([]byte(nil), full...), 1),
		"second archive":     append(append([]byte(nil), full...), full...),
	} {
		t.Run(name, func(t *testing.T) {
			scanFails(t, data, diff.ScanOptions{}, diff.ErrInvalidInput)
		})
	}
	gz := scanGzip(t, full)
	badCRC := append([]byte(nil), gz...)
	badCRC[len(badCRC)-8] ^= 0x80
	err := scanFails(t, badCRC, diff.ScanOptions{}, diff.ErrInvalidInput)
	if !errors.Is(err, gzip.ErrChecksum) {
		t.Fatal("gzip checksum error identity lost")
	}
	for _, n := range []int{2, len(gz) - 1, len(gz) - 8} {
		scanFails(t, gz[:n], diff.ScanOptions{}, diff.ErrInvalidInput)
	}
	// Members may divide a single tar at any decoded byte boundary.
	for _, split := range []int{0, 300, 514, 1024, len(full)} {
		joined := append(scanGzip(t, full[:split]), scanGzip(t, full[split:])...)
		if !reflect.DeepEqual(scanOK(t, joined, diff.ScanOptions{}), scanOK(t, full, diff.ScanOptions{})) {
			t.Fatalf("gzip split %d changed manifest", split)
		}
	}
	scanOK(t, append(gz, scanGzip(t, make([]byte, 513))...), diff.ScanOptions{})
	scanFails(t, append(append([]byte(nil), gz...), scanGzip(t, full)...), diff.ScanOptions{}, diff.ErrInvalidInput)
	// Corrupt a final empty member: tar EOF precedes the gzip error.
	emptyGzip := scanGzip(t, nil)
	emptyGzip[len(emptyGzip)-8] ^= 1
	err = scanFails(t, append(append([]byte(nil), gz...), emptyGzip...), diff.ScanOptions{}, diff.ErrInvalidInput)
	if !errors.Is(err, gzip.ErrChecksum) {
		t.Fatal("gzip error after tar EOF was lost")
	}
}

func TestScanGODEBUGAndOrderIndependence(t *testing.T) {
	entries := []testfixture.Entry{scanEntry("z", tar.TypeReg, "hello"), scanEntry(`a\b`, tar.TypeReg, "")}
	one := scanArchive(t, entries...)
	entries[0], entries[1] = entries[1], entries[0]
	two := scanArchive(t, entries...)
	want := scanOK(t, one, diff.ScanOptions{})
	for _, debug := range []string{"tarinsecurepath=0", "tarinsecurepath=1"} {
		t.Setenv("GODEBUG", debug)
		if !reflect.DeepEqual(scanOK(t, two, diff.ScanOptions{}), want) {
			t.Fatal("GODEBUG/order affected manifest")
		}
		scanFails(t, scanArchive(t, scanEntry("../invalid", tar.TypeReg, "")), diff.ScanOptions{}, diff.ErrInvalidInput)
	}
}

func TestScanHardlinkHeaderSizeAndActualPayload(t *testing.T) {
	base := scanArchive(t,
		testfixture.Entry{Header: tar.Header{Name: "alias", Typeflag: tar.TypeLink, Linkname: "target", Size: 1, Mode: 0777}},
		testfixture.Entry{Header: tar.Header{Name: "target", Typeflag: tar.TypeReg, Size: 5, Mode: 0600}, Data: []byte("hello")},
	)
	m := scanOK(t, base, diff.ScanOptions{})
	if m.Entries[0].Size != 5 || m.Entries[0].Mode != 0600 || m.Entries[0].SHA256 != helloSHA {
		t.Fatalf("hardlink header was treated as distinct inode metadata: %#v", m)
	}
	// Header-only metadata is not a payload. Actually inserted bytes are
	// unsupported because they occupy the next required tar-header position.
	actualPayload := append(append(append([]byte(nil), base[:512]...), byte('x')), base[512:]...)
	scanFails(t, actualPayload, diff.ScanOptions{}, diff.ErrInvalidInput)
}

func TestScanEveryPAXExtensionIsAudited(t *testing.T) {
	second := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{"VENDOR.second": "second secret"}}})
	for _, tc := range []struct {
		key      string
		category error
	}{
		{"GNU.sparse.major", diff.ErrUnsupportedInput},
		{"VENDOR.ba\xff", diff.ErrInvalidInput},
		{"VENDOR.first", nil},
	} {
		placeholder := strings.Repeat("v", len(tc.key))
		first := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{placeholder: "2"}}})
		first = bytes.ReplaceAll(first, []byte(placeholder+"="), []byte(tc.key+"="))
		input := append(append([]byte(nil), first[:1024]...), second...)
		if tc.category != nil {
			scanFails(t, input, diff.ScanOptions{}, tc.category)
		} else {
			m := scanOK(t, input, diff.ScanOptions{})
			want := []diff.Warning{
				{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "VENDOR.first"},
				{Path: "file", Code: diff.WarningUncomparedMetadata, Key: "VENDOR.second"},
			}
			if !reflect.DeepEqual(m.Warnings, want) {
				t.Fatalf("stacked PAX warnings = %#v", m.Warnings)
			}
		}
	}
	// The standard library still applies final PAX size, GNU long names, and
	// PAX long names while the audit observes their physical records.
	long := strings.Repeat("n", 170)
	longArchive := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: long, Typeflag: tar.TypeReg, Size: 5, Format: tar.FormatGNU}, Data: []byte("hello")})
	if m := scanOK(t, longArchive, diff.ScanOptions{}); m.Entries[0].SHA256 != helloSHA {
		t.Fatal("extension observation affected GNU payload framing")
	}
	// PAX size overrides the physical regular header size, which may be zero.
	withSize := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Size: 5, Format: tar.FormatPAX, PAXRecords: map[string]string{"size": "5"}}, Data: []byte("hello")})
	physical := append([]byte(nil), withSize...)
	hdr := patchTarHeader(t, physical[1024:], func(h []byte) { copy(h[124:136], "00000000000\x00") })
	copy(physical[1024:], hdr)
	if m := scanOK(t, physical, diff.ScanOptions{}); m.Entries[0].SHA256 != helloSHA {
		t.Fatal("PAX effective size was lost")
	}
}

func TestScanAuditRejectsFailedPAXMerge(t *testing.T) {
	second := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{"VENDOR.second": "second"}}})
	for _, field := range []string{"uid", "gid", "size", "mtime", "atime", "ctime"} {
		for _, key := range []string{"GNU.sparse.major", "VENDOR.ba\xff", "VENDOR.first"} {
			t.Run(field+"/"+fmt.Sprintf("%q", key), func(t *testing.T) {
				// Use equal-length vendor placeholders because tar.Writer omits
				// standard PAX fields that conflict with its Header values.
				fieldPlaceholder := strings.Repeat("n", len(field))
				keyPlaceholder := strings.Repeat("v", len(key))
				first := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{
					fieldPlaceholder: "not-a-number", keyPlaceholder: "2",
				}}})
				first = bytes.ReplaceAll(first, []byte(fieldPlaceholder+"="), []byte(field+"="))
				first = bytes.ReplaceAll(first, []byte(keyPlaceholder+"="), []byte(key+"="))
				input := append(append([]byte(nil), first[:1024]...), second...)
				err := scanFails(t, input, diff.ScanOptions{}, diff.ErrInvalidInput)
				if strings.Contains(err.Error(), "not-a-number") {
					t.Fatal("error leaked metadata value")
				}
			})
		}
	}
}

func TestScanAuditAcceptsEmptyPAXMapsAndValues(t *testing.T) {
	entry := scanArchive(t, scanEntry("file", tar.TypeReg, ""))
	// An empty x header still decodes to a non-nil, empty PAX map.
	empty := scanArchive(t, scanEntry("extension", tar.TypeReg, ""))
	empty = patchTarHeader(t, empty, func(h []byte) { h[156] = tar.TypeXHeader })
	scanOK(t, append(append([]byte(nil), empty[:512]...), entry...), diff.ScanOptions{})
	for _, key := range []string{"uid", "gid", "size", "mtime", "atime", "ctime", "VENDOR.empty"} {
		placeholder := strings.Repeat("v", len(key))
		first := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Format: tar.FormatPAX, PAXRecords: map[string]string{placeholder: ""}}})
		first = bytes.ReplaceAll(first, []byte(placeholder+"="), []byte(key+"="))
		m := scanOK(t, append(append([]byte(nil), first[:1024]...), entry...), diff.ScanOptions{})
		if key == "VENDOR.empty" {
			want := []diff.Warning{{Path: "file", Code: diff.WarningUncomparedMetadata, Key: key}}
			if !reflect.DeepEqual(m.Warnings, want) {
				t.Fatalf("empty metadata warning = %#v", m.Warnings)
			}
		} else if len(m.Warnings) != 0 {
			t.Fatalf("standard empty value produced warning: %#v", m.Warnings)
		}
	}
}

func TestScanMalformedDeflateHasTypedCategory(t *testing.T) {
	// Fixed gzip header followed by reserved DEFLATE block type 3.
	input := []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255, 0x06, 0, 0, 0, 0, 0, 0, 0, 0}
	err := scanFails(t, input, diff.ScanOptions{}, diff.ErrInvalidInput)
	var corrupt flate.CorruptInputError
	if !errors.As(err, &corrupt) {
		t.Fatal("underlying DEFLATE error identity lost")
	}
}

func TestScanLongTargetsAndDefaultPathBoundary(t *testing.T) {
	for _, format := range []tar.Format{tar.FormatGNU, tar.FormatPAX} {
		target := strings.Repeat("a", 200)
		input := scanArchive(t,
			testfixture.Entry{Header: tar.Header{Name: "alias", Typeflag: tar.TypeLink, Linkname: target, Format: format}},
			testfixture.Entry{Header: tar.Header{Name: "symlink", Typeflag: tar.TypeSymlink, Linkname: "/../" + target, Format: format}},
			testfixture.Entry{Header: tar.Header{Name: target, Typeflag: tar.TypeReg, Format: format}},
		)
		m := scanOK(t, input, diff.ScanOptions{})
		if m.Entries[1].LinkTarget != target || m.Entries[2].LinkTarget != "/../"+target {
			t.Fatalf("long targets = %#v", m)
		}
	}
	for _, n := range []int{4096, 4097} {
		input := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: strings.Repeat("p", n), Typeflag: tar.TypeReg, Format: tar.FormatPAX}})
		if n == 4096 {
			scanOK(t, input, diff.ScanOptions{})
		} else {
			scanFails(t, input, diff.ScanOptions{}, diff.ErrLimitExceeded)
		}
	}
}

func TestScanSignedOwnershipAndModeMask(t *testing.T) {
	input := scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Mode: 0107755, Uid: -7, Gid: -8, Format: tar.FormatGNU}})
	e := scanOK(t, input, diff.ScanOptions{}).Entries[0]
	if e.UID != -7 || e.GID != -8 || e.Mode != 07755 {
		t.Fatalf("numeric metadata = %#v", e)
	}
	input = scanArchive(t, testfixture.Entry{Header: tar.Header{Name: "device", Typeflag: tar.TypeChar, Devmajor: -1, Format: tar.FormatGNU}})
	scanFails(t, input, diff.ScanOptions{}, diff.ErrInvalidInput)
}

func TestScanLongForwardHardlinkChain(t *testing.T) {
	entries := make([]testfixture.Entry, 2049)
	for i := 0; i < 2048; i++ {
		entries[i] = testfixture.Entry{Header: tar.Header{Name: fmt.Sprintf("link%04d", i), Typeflag: tar.TypeLink, Linkname: fmt.Sprintf("link%04d", i+1)}}
	}
	entries[2048] = testfixture.Entry{Header: tar.Header{Name: "link2048", Typeflag: tar.TypeReg, Mode: 04751, Uid: 10, Gid: 20, Size: 5}, Data: []byte("hello")}
	m := scanOK(t, scanArchive(t, entries...), diff.ScanOptions{})
	for _, e := range m.Entries {
		if e.Size != 5 || e.SHA256 != helloSHA || e.Mode != 04751 || e.UID != 10 || e.GID != 20 {
			t.Fatalf("effective chain data = %#v", e)
		}
	}
}

func TestScanNilInputs(t *testing.T) {
	if _, err := diff.Scan(nil, bytes.NewReader(nil), diff.ScanOptions{}); !errors.Is(err, diff.ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := diff.Scan(context.Background(), nil, diff.ScanOptions{}); !errors.Is(err, diff.ErrInvalidInput) {
		t.Fatal(err)
	}
}
