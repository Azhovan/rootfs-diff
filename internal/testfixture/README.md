# Archive fixtures

This internal test helper writes small archives in memory with Go's standard
library. It does not extract files, resolve host users/groups, or call production
scan/compare code. Author expected manifests and reports independently.

```go
archive, err := testfixture.Tar(testfixture.Entry{
    Header: tar.Header{
        Name: "etc/device.conf", Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
        Mode: 06750, Uid: 123, Gid: 456, Size: 4,
    },
    Data: []byte("abc\n"),
})
```

Set `Header.Size` explicitly for payload records. Headers are encoded with the
normalization and format constraints of `archive/tar`; payloads inconsistent with
the encoded size return errors. Use `tar.FormatGNU` or `tar.FormatPAX` for long names,
`PAXRecords` or `Xattrs` for vendor metadata, `Linkname` for symlinks/hardlinks,
and `Devmajor`/`Devminor` for devices. Entry order is the argument order, allowing
forward hardlinks and duplicate-path fixtures.

`Gzip(archive)` creates a deterministic single gzip member without a filename or
timestamp. Concatenate member byte slices to test multi-member input.

`Patch(data, offset, replacement)` returns a copy with the requested bytes
replaced and rejects out-of-bounds patches. It supports corrupted tar checksums
or gzip trailers. Pass a prefix such as `archive[:514]` to create truncation;
append bytes or whole archives to exercise trailing data. Unsupported records
that `archive/tar.Writer` refuses to encode need deliberate raw byte fixtures.

The builder buffers all fixture bytes. Use small payloads or a custom streaming
test reader for resource-limit tests.
