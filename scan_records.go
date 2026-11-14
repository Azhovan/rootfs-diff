package rootfsdiff

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
)

// scanRecordReader observes physical extension records while archive/tar owns
// parsing and effective header semantics. Next may replace a previous local PAX
// map with a later map; auditing each extension prevents discarded sparse or
// uncompared metadata from escaping validation. PAX decoding remains entirely
// in archive/tar, including its bounded extension handling.
//
// Reads stop at header/body boundaries. The caller supplies the effective body
// size immediately after Next returns, accounting for PAX sizes and legacy
// directory type normalization without a second header or numeric parser.
type scanRecordReader struct {
	r       io.Reader
	header  [512]byte
	filled  int
	data    int64
	padding int64
	pax     bool
	payload []byte
	keys    map[string]struct{}
	pending bool
}

func (r *scanRecordReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.data > 0 {
		if int64(len(p)) > r.data {
			p = p[:int(r.data)]
		}
		n, err := r.r.Read(p)
		r.data -= int64(n)
		if r.pax {
			r.payload = append(r.payload, p[:n]...)
			if r.data == 0 {
				if auditErr := r.auditPAX(); auditErr != nil {
					return n, auditErr
				}
			}
		}
		return n, err
	}
	if r.padding > 0 {
		if int64(len(p)) > r.padding {
			p = p[:int(r.padding)]
		}
		n, err := r.r.Read(p)
		r.padding -= int64(n)
		return n, err
	}
	if len(p) > len(r.header)-r.filled {
		p = p[:len(r.header)-r.filled]
	}
	n, err := r.r.Read(p)
	copy(r.header[r.filled:], p[:n])
	r.filled += n
	if r.filled != len(r.header) {
		return n, err
	}
	r.filled = 0
	switch r.header[156] {
	case tar.TypeGNUSparse:
		return n, scanUnsupported("", "GNU sparse entries are unsupported")
	case tar.TypeXGlobalHeader:
		return n, scanUnsupported("", "global PAX headers are unsupported")
	case tar.TypeXHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
		// Decode the physical extension's size with archive/tar. A regular
		// type exposes its header without trying to read its missing body.
		hdr := scanHeaderType(r.header, tar.TypeReg)
		h, parseErr := tar.NewReader(bytes.NewReader(hdr[:])).Next()
		if parseErr != nil && parseErr != tar.ErrInsecurePath {
			return n, parseErr
		}
		// Delegate the allocation bound to archive/tar's extension reader.
		// It returns ErrFieldTooLong once 1 MiB is exceeded. Only collect
		// payloads it could accept; oversized ones need no second parse.
		r.pax = r.header[156] == tar.TypeXHeader && h.Size <= 1<<20
		r.data, r.padding = h.Size, (512-h.Size%512)%512
		r.payload = nil
		r.pending = true
		if r.pax && r.data == 0 {
			if auditErr := r.auditPAX(); auditErr != nil {
				return n, auditErr
			}
		}
	}
	return n, err
}

func (r *scanRecordReader) entryBody(h *tar.Header) {
	r.pax, r.payload = false, nil
	r.data, r.padding = 0, 0
	if h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeRegA {
		r.data, r.padding = h.Size, (512-h.Size%512)%512
	}
}

func (r *scanRecordReader) auditPAX() error {
	// A global type exposes the parsed PAX map instead of hiding it. It is
	// only used in this in-memory parser, never accepted from the archive.
	hdr := scanHeaderType(r.header, tar.TypeXGlobalHeader)
	h, err := tar.NewReader(io.MultiReader(bytes.NewReader(hdr[:]), bytes.NewReader(r.payload))).Next()
	r.payload = nil
	if err != nil && err != tar.ErrInsecurePath {
		return err
	}
	warnings := make(map[Warning]struct{})
	if err := scanMetadata(h, "", warnings); err != nil {
		return err
	}
	if r.keys == nil {
		r.keys = make(map[string]struct{}, len(warnings))
	}
	for w := range warnings {
		r.keys[w.Key] = struct{}{}
	}
	return nil
}

func scanHeaderType(header [512]byte, kind byte) [512]byte {
	header[156] = kind
	for i := 148; i < 156; i++ {
		header[i] = ' '
	}
	var sum int
	for _, b := range header {
		sum += int(b)
	}
	copy(header[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return header
}
