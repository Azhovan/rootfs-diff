package rootfsdiff

import (
	"archive/tar"
	"bufio"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// Scan reads an uncompressed or gzip-compressed tar stream, detected by magic
// bytes. It never extracts entries, resolves host links, or closes r. Only
// explicit entries are returned, sorted by canonical POSIX path. Payloads are
// hashed in bounded chunks and are not retained.
//
// Zero options select documented defaults. The decoded-byte budget includes
// headers, extensions and padding; the logical-entry budget includes ignored
// root directory records. Path limits apply to decoded names before path
// normalization. Hardlinks retain their canonical target and inherit effective
// data from their terminal regular entry. Uncompared PAX metadata produces
// sorted warnings containing keys, never values. Standard metadata exclusions
// are available through MetadataExclusions for the final report.
//
// Scan checks ctx between reads and during link resolution. Cancellation cannot
// interrupt an arbitrary reader already blocked in Read. The caller owns any
// deadline or Close needed to unblock that reader. On error Scan returns a zero
// Manifest, never a successful partial result. Independent calls share no state.
func Scan(ctx context.Context, r io.Reader, opts ScanOptions) (result Manifest, resultErr error) {
	if ctx == nil || r == nil {
		return Manifest{}, invalid("scan", "", "context and reader must be non-nil")
	}
	opts, err := opts.WithDefaults()
	if err != nil {
		return Manifest{}, err
	}
	raw := &scanReader{ctx: ctx, r: r}
	var budget *scanReader
	defer func() {
		for _, reader := range []*scanReader{raw, budget} {
			if reader != nil && reader.failure != nil && !errors.Is(resultErr, reader.failure) {
				resultErr = errors.Join(resultErr, scanReadError("read archive", "", reader.failure))
			}
		}
		if resultErr != nil {
			result = Manifest{}
		}
	}()
	buffered := bufio.NewReader(raw)
	magic, err := buffered.Peek(2)
	if err != nil && err != io.EOF {
		return Manifest{}, scanReadError("detect compression", "", err)
	}
	var decoded io.Reader = buffered
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(buffered)
		if err != nil {
			return Manifest{}, scanReadError("read gzip header", "", err)
		}
		// Close releases decompressor state; it never closes the input reader.
		defer gz.Close()
		decoded = gz // The default multistream mode validates every gzip member.
	}
	budget = &scanReader{ctx: ctx, r: decoded, maximum: opts.MaxArchiveBytes, limited: true}
	records := &scanRecordReader{r: budget}
	tr := tar.NewReader(records)
	entries := make(map[string]Entry)
	warnings := make(map[Warning]struct{})
	var count int64
	buf := make([]byte, 32*1024)
	for {
		h, err := tr.Next()
		// archive/tar's host-dependent check is advisory. Always apply our own
		// POSIX rules, independently of GODEBUG and the host path implementation.
		if errors.Is(err, tar.ErrInsecurePath) && h != nil {
			err = nil
		}
		if err == io.EOF {
			if records.pending {
				return Manifest{}, invalid("read tar header", "", "metadata extension has no following entry")
			}
			break
		}
		if err != nil {
			return Manifest{}, scanReadError("read tar header", "", err)
		}
		records.entryBody(h)
		if count == opts.MaxEntries {
			return Manifest{}, scanLimit("max_entries", opts.MaxEntries, count, nil)
		}
		count++
		if h.Typeflag == tar.TypeXGlobalHeader {
			return Manifest{}, scanUnsupported(h.Name, "global PAX headers are unsupported")
		}
		if h.Typeflag == tar.TypeGNUSparse {
			return Manifest{}, scanUnsupported(h.Name, "GNU sparse entries are unsupported")
		}
		p, err := scanPath(h.Name, opts.MaxPathBytes)
		if err != nil {
			return Manifest{}, err
		}
		for k := range records.keys {
			warnings[Warning{Path: p, Code: WarningUncomparedMetadata, Key: k}] = struct{}{}
		}
		records.keys, records.pending = nil, false
		if err := scanMetadata(h, p, warnings); err != nil {
			return Manifest{}, err
		}
		e := Entry{Path: p, Mode: uint32(h.Mode & 07777), UID: int64(h.Uid), GID: int64(h.Gid)}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			e.Kind = KindRegular
		case tar.TypeDir:
			e.Kind = KindDirectory
		case tar.TypeSymlink:
			e.Kind = KindSymlink
		case tar.TypeLink:
			e.Kind = KindHardlink
		case tar.TypeChar:
			e.Kind = KindCharDevice
		case tar.TypeBlock:
			e.Kind = KindBlockDevice
		case tar.TypeFifo:
			e.Kind = KindFIFO
		default:
			return Manifest{}, scanUnsupported(p, fmt.Sprintf("tar entry type %#x is unsupported", h.Typeflag))
		}
		if p == "." && e.Kind != KindDirectory {
			return Manifest{}, invalid("scan entry", p, "root entry must be a directory")
		}
		// Directory sizes are deliberately ignored. Tar defines directories as
		// header-only records even when their size metadata is nonzero. Actual
		// bytes inserted after such a header must still parse as the next header.
		if e.Kind != KindRegular && e.Kind != KindDirectory && e.Kind != KindHardlink && h.Size != 0 {
			return Manifest{}, scanUnsupported(p, "non-regular payload bytes are unsupported")
		}
		if e.Kind == KindSymlink || e.Kind == KindHardlink {
			if int64(len(h.Linkname)) > opts.MaxPathBytes {
				return Manifest{}, &Error{Op: "scan link", Path: p, Err: &LimitError{Limit: "max_path_bytes", Maximum: opts.MaxPathBytes, Actual: int64(len(h.Linkname))}}
			}
			if !utf8.ValidString(h.Linkname) {
				return Manifest{}, invalid("scan link", p, "link target must be UTF-8")
			}
			e.LinkTarget = h.Linkname
			if e.Kind == KindHardlink {
				e.LinkTarget, err = scanPath(h.Linkname, opts.MaxPathBytes)
				if err != nil {
					return Manifest{}, &Error{Op: "scan hardlink", Path: p, Err: err}
				}
				if e.LinkTarget == "." {
					return Manifest{}, invalid("scan hardlink", p, "hardlink target must name a regular entry, not root")
				}
			}
		}
		if e.Kind == KindCharDevice || e.Kind == KindBlockDevice {
			if h.Devmajor < 0 || h.Devminor < 0 {
				return Manifest{}, invalid("scan device", p, "device numbers must be nonnegative")
			}
			e.DeviceMajor, e.DeviceMinor = h.Devmajor, h.Devminor
		}
		if p == "." { // Explicit root metadata has no manifest entry.
			continue
		}
		if _, exists := entries[p]; exists {
			return Manifest{}, invalid("scan entry", p, "duplicate normalized path")
		}
		if e.Kind == KindRegular {
			if h.Size > opts.MaxFileBytes {
				return Manifest{}, &Error{Op: "scan payload", Path: p, Err: &LimitError{Limit: "max_file_bytes", Maximum: opts.MaxFileBytes, Actual: h.Size}}
			}
			digest := sha256.New()
			// Avoid io.Copy optimizations that could bypass read-boundary checks.
			var size int64
			for {
				if err := ctx.Err(); err != nil {
					return Manifest{}, &Error{Op: "scan payload", Path: p, Err: err}
				}
				n, readErr := tr.Read(buf)
				if int64(n) > math.MaxInt64-size {
					return Manifest{}, invalid("scan payload", p, "payload size overflows int64")
				}
				size += int64(n)
				_, _ = digest.Write(buf[:n])
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					return Manifest{}, scanReadError("read tar payload", p, readErr)
				}
			}
			if size != h.Size {
				return Manifest{}, invalid("scan payload", p, "payload size differs from header")
			}
			e.Size, e.SHA256 = size, hex.EncodeToString(digest.Sum(nil))
		}
		entries[p] = e
	}
	// Tar terminators do not validate gzip trailers or consume all padding.
	// Complete the decoded stream; a second archive is nonzero trailing data.
	for {
		n, err := budget.Read(buf)
		for _, b := range buf[:n] {
			if b != 0 {
				return Manifest{}, invalid("scan trailing data", "", "only zero decoded padding is allowed after tar EOF")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Manifest{}, scanReadError("read archive EOF", "", err)
		}
	}
	// ReadFull in archive/tar can suppress an error returned with a full read.
	// Retain those errors even when they coincide with a tar terminator block.
	for _, r := range []*scanReader{budget, raw} {
		if r.failure != nil {
			return Manifest{}, scanReadError("read archive", "", r.failure)
		}
	}
	if err := scanResolve(ctx, entries); err != nil {
		return Manifest{}, err
	}
	m := Manifest{Entries: make([]Entry, 0, len(entries)), Warnings: make([]Warning, 0, len(warnings))}
	for _, e := range entries {
		m.Entries = append(m.Entries, e)
	}
	for w := range warnings {
		m.Warnings = append(m.Warnings, w)
	}
	slices.SortFunc(m.Entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(m.Warnings, compareWarning)
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, &Error{Op: "scan", Err: err}
	}
	return m, nil
}

func scanPath(p string, maximum int64) (string, error) {
	if int64(len(p)) > maximum {
		return "", &Error{Op: "scan path", Err: &LimitError{Limit: "max_path_bytes", Maximum: maximum, Actual: int64(len(p))}}
	}
	if p == "" || !utf8.ValidString(p) || strings.HasPrefix(p, "/") {
		return "", invalid("scan path", p, "expected a nonempty UTF-8 root-relative path")
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, part := range parts {
		if part == ".." {
			return "", invalid("scan path", p, "parent '..' segments are forbidden")
		}
		if part != "" && part != "." {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return ".", nil
	}
	return strings.Join(out, "/"), nil
}

func scanMetadata(h *tar.Header, p string, warnings map[Warning]struct{}) error {
	keys := make([]string, 0, len(h.PAXRecords))
	for k := range h.PAXRecords {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := h.PAXRecords[k]
		if !utf8.ValidString(k) {
			return invalid("scan metadata", p, "PAX metadata key must be UTF-8")
		}
		if k == "SUN.holesdata" || strings.HasPrefix(k, "GNU.sparse.") || strings.HasPrefix(k, "SCHILY.sparse.") || k == "SCHILY.realsize" || (k == "SCHILY.filetype" && v == "sparse") {
			return scanUnsupported(p, "sparse metadata "+fmt.Sprintf("%q", k)+" is unsupported")
		}
		switch k {
		case "path", "linkpath", "size", "uid", "gid", "mtime", "atime", "ctime", "uname", "gname":
			continue
		}
		warnings[Warning{Path: p, Code: WarningUncomparedMetadata, Key: k}] = struct{}{}
	}
	return nil
}

func scanResolve(ctx context.Context, entries map[string]Entry) error {
	// Visit paths in stable order so invalid link diagnostics are deterministic.
	paths := make([]string, 0, len(entries))
	for p := range entries {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	resolved := make(map[string]bool)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return &Error{Op: "resolve hardlinks", Err: err}
		}
		if entries[path].Kind != KindHardlink || resolved[path] {
			continue
		}
		var chain []string
		seen := make(map[string]bool)
		p := path
		var terminal Entry
		for {
			if err := ctx.Err(); err != nil {
				return &Error{Op: "resolve hardlink", Path: path, Err: err}
			}
			e, ok := entries[p]
			if !ok {
				return invalid("resolve hardlink", path, "missing target "+fmt.Sprintf("%q", p))
			}
			if e.Kind == KindRegular || resolved[p] {
				terminal = e
				break
			}
			if e.Kind != KindHardlink {
				return invalid("resolve hardlink", path, "target does not end in a regular entry")
			}
			if seen[p] {
				return invalid("resolve hardlink", path, "hardlink cycle")
			}
			seen[p] = true
			chain = append(chain, p)
			p = e.LinkTarget
		}
		for _, alias := range chain {
			e := entries[alias]
			e.Size, e.SHA256 = terminal.Size, terminal.SHA256
			e.Mode, e.UID, e.GID = terminal.Mode, terminal.UID, terminal.GID
			entries[alias] = e
			resolved[alias] = true
		}
	}
	return nil
}

func scanUnsupported(path, detail string) error {
	return &Error{Op: "scan entry", Path: path, Kind: ErrorUnsupportedInput, Err: errors.New(detail)}
}

func scanReadError(op, path string, err error) error {
	kind := ErrorKind("")
	var deflateErr flate.CorruptInputError
	if errors.As(err, &deflateErr) || errors.Is(err, tar.ErrHeader) || errors.Is(err, tar.ErrFieldTooLong) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, gzip.ErrHeader) || errors.Is(err, gzip.ErrChecksum) {
		kind = ErrorInvalidInput
	}
	return &Error{Op: op, Path: path, Kind: kind, Err: err}
}

// scanReader checks cancellation/no-progress, records errors that parsers may
// swallow on full reads, and optionally budgets every decoded byte. It does not
// implement ReaderFrom or WriterTo, so no fast path bypasses its Read method.
type scanReader struct {
	ctx     context.Context
	r       io.Reader
	maximum int64
	used    int64
	limited bool
	empty   int
	failure error
}

func (r *scanReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.limited && r.used == r.maximum {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n != 0 {
			err = scanLimit("max_archive_bytes", r.maximum, r.used, err)
			r.failure = err
			return 0, err
		}
		return 0, r.record(0, err)
	}
	if r.limited && int64(len(p)) > r.maximum-r.used {
		p = p[:int(r.maximum-r.used)]
	}
	n, err := r.r.Read(p)
	if r.limited {
		r.used += int64(n) // n is bounded by the remaining budget above.
	}
	return n, r.record(n, err)
}

func (r *scanReader) record(n int, err error) error {
	if n == 0 && err == nil {
		r.empty++
		if r.empty >= 100 {
			err = io.ErrNoProgress
		}
	} else {
		r.empty = 0
	}
	if err != nil && err != io.EOF && r.failure == nil {
		r.failure = err
	}
	return err
}

func scanLimit(name string, maximum, observed int64, cause error) error {
	// Counts are checked even when a caller selects MaxInt64 as the budget.
	if observed < math.MaxInt64 {
		observed++
	} else {
		cause = errors.Join(cause, errors.New("observed count exceeds int64"))
	}
	return &LimitError{Limit: name, Maximum: maximum, Actual: observed, Err: cause}
}
