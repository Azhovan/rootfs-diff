# Archive fixtures

This internal test helper writes small archives in memory using Go's standard
library. It does not extract files, resolve host users or groups, or call the
production scanner or comparator. Write expected manifests and reports
independently of the code being tested.

```go
archive, err := testfixture.Tar(testfixture.Entry{
    Header: tar.Header{
        Name: "etc/device.conf", Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
        Mode: 06750, Uid: 123, Gid: 456, Size: 4,
    },
    Data: []byte("abc\n"),
})
```

Set `Header.Size` explicitly for entries with payloads. `archive/tar` encodes
headers using its normal normalization and format rules. Payloads that do not
match the encoded size return errors. Use `tar.FormatGNU` or `tar.FormatPAX` for long names,
`PAXRecords` or `Xattrs` for vendor metadata, `Linkname` for symlinks/hardlinks,
and `Devmajor`/`Devminor` for devices. Entries are written in argument order,
so tests can include forward hardlink references and duplicate paths.

`Gzip(archive)` creates a single gzip member without a filename or timestamp,
producing the same bytes each time. To test multiple members, concatenate their
byte slices.

`Patch(data, offset, replacement)` returns a copy with the requested bytes
replaced. It rejects patches outside the slice bounds and can be used to corrupt
tar checksums or gzip trailers. Use a prefix such as `archive[:514]` to create
truncated input, or append bytes or whole archives to test trailing data. To test
records that `archive/tar.Writer` refuses to encode, construct raw bytes explicitly.

The builder buffers all fixture bytes. Use small payloads or a custom streaming
test reader for resource-limit tests.
