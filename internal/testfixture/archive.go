// Package testfixture builds small in-memory archives for tests. It does not use
// rootfsdiff parsing or comparison code, and never creates archive entries on the
// host filesystem. Expected manifests and reports must be authored separately.
package testfixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
)

// Entry describes one tar record. Header is encoded by archive/tar with its
// usual normalization and format constraints. Callers set Size to the length of
// Data for payload records; no metadata is derived from the host.
// Format can force USTAR, GNU, or PAX encoding; PAXRecords and Xattrs can express
// vendor metadata. Links and devices use the ordinary tar.Header fields.
type Entry struct {
	Header tar.Header
	Data   []byte
}

// Tar returns an archive in the supplied entry order, including tar terminators.
// It does not modify caller-supplied headers or data. Header encoding errors and
// payloads inconsistent with the encoded size return errors.
func Tar(entries ...Entry) ([]byte, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for i, entry := range entries {
		header := entry.Header
		header.PAXRecords = cloneMap(header.PAXRecords)
		header.Xattrs = cloneMap(header.Xattrs)
		if err := w.WriteHeader(&header); err != nil {
			return nil, fmt.Errorf("fixture entry %d header: %w", i, err)
		}
		if _, err := w.Write(entry.Data); err != nil {
			return nil, fmt.Errorf("fixture entry %d payload: %w", i, err)
		}
		if err := w.Flush(); err != nil {
			return nil, fmt.Errorf("fixture entry %d payload: %w", i, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("fixture archive: %w", err)
	}
	return buf.Bytes(), nil
}

// Gzip returns a single gzip member with no filename, comment, or timestamp.
// Repeated calls on the same input produce identical bytes with the pinned Go
// toolchain. Concatenating results creates a multi-member gzip fixture.
func Gzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.OS = 255
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("fixture gzip payload: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("fixture gzip trailer: %w", err)
	}
	return buf.Bytes(), nil
}

// Patch copies data and replaces bytes at offset. It can corrupt checksums,
// headers, payloads, or gzip trailers without modifying a shared fixture. For
// truncation, pass a prefix of an archive directly to the code under test.
func Patch(data []byte, offset int, replacement []byte) ([]byte, error) {
	if offset < 0 || offset > len(data) || len(replacement) > len(data)-offset {
		return nil, fmt.Errorf("fixture patch: offset %d and length %d exceed %d bytes", offset, len(replacement), len(data))
	}
	out := append([]byte(nil), data...)
	copy(out[offset:], replacement)
	return out, nil
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
