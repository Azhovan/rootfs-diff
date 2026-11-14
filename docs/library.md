# Go library

Import `github.com/Azhovan/rootfs-diff` as package `rootfsdiff`. The root package owns scanning, comparison, policy evaluation and the shared models. The separate public `github.com/Azhovan/rootfs-diff/report` package writes reports; core code does not depend on rendering or process behavior.

The [runnable library example](../examples/library/README.md) imports the public packages and demonstrates the complete scan-to-report flow. See [module setup](installation.md#use-the-go-module) before adding the dependency to your own module.

## Operations

```go
Scan(context.Context, io.Reader, ScanOptions) (Manifest, error)
Compare(before, after Manifest, opts CompareOptions) (Diff, error)
ParsePolicy(io.Reader) (Policy, error)
Evaluate(after Manifest, diff Diff, policy Policy) ([]Violation, error)
```

`Scan` consumes one tar/tar.gz stream and returns explicit canonical entries and metadata warnings. Use `ScanOptions{}` for defaults or positive field overrides. `Compare` validates both manifests and applies `CompareOptions{IgnorePaths: ..., IgnorePrefixes: ...}` symmetrically. `Evaluate` uses the complete candidate manifest for required paths and the supplied diff for net growth after ignores. Keep these values from the same comparison; the evaluator cannot reconstruct an ignored baseline from the diff.

Use `ParsePolicy` for untrusted policy input. Ordinary `encoding/json` decoding plus `Policy.Validate` does not supply duplicate-key, unknown-key, required-field, trailing-input and 1 MiB parser checks. `Policy.MaxTotalGrowthBytes` is `*int64`: nil disables the gate, and a pointer to zero enables a no-positive-growth gate.

The public renderer's signature is:

```go
report.Write(io.Writer, rootfsdiff.Report, report.Format) error
```

Use `report.Text`, `report.JSON` or `report.Markdown`; a zero/unknown format is unsupported. Assemble a report from the returned diff and violations:

```go
result := rootfsdiff.Report{
    SchemaVersion:      rootfsdiff.SchemaVersion,
    Summary:            diff.Summary,
    Changes:            diff.Changes,
    PositiveGrowth:     diff.PositiveGrowth,
    Violations:         violations,
    Warnings:           diff.Warnings,
    MetadataExclusions: rootfsdiff.MetadataExclusions(),
}
```

`MetadataExclusions()` returns a fresh slice with the full fixed list. `Report.Validate` requires it. Validation precedes rendering; output failures still need handling, since a writer may fail after accepting part of a report. The renderer encodes the report in memory, then writes it once; it does not retain archive payloads. The library returns violations as data rather than exiting or inferring a CI failure. The caller decides how those violations and `Diff.Changes` affect its own process.

## Ownership, validation and concurrency

Readers and writers belong to the caller. Scanning/policy parsing do not close their readers; rendering does not close its writer. Open and close files in the calling application, and handle both operation errors and any relevant close errors there. Stream payloads are consumed, not retained or extracted. To scan again, supply a fresh reader or rewind a seekable one.

Scan checks context cancellation between bounded reads. Context cancellation cannot unblock an arbitrary `Read` already in progress. Compare, parse, evaluate and render do not take a context; input budgets and caller-provided I/O behavior bound their work.

Operations do not mutate caller models/options, print, exit or use mutable global options. Independent calls can run concurrently with independent readers/writers and models that callers do not concurrently mutate. Slices and pointers in model structs remain mutable Go values; the API does not synchronize caller writes. Avoid concurrently writing a shared destination or reading the same advancing stream.

Caller-built manifests may list entries/warnings in any order. Model paths must already be canonical; `ValidatePath` validates without normalization, host filesystem access or scan length caps. Complete manifest validation checks field applicability, unique paths, explicit ancestors, hardlink closure/effective metadata and checked totals. Produced manifests/diffs are sorted; report/diff validation additionally enforces required output ordering and cross-field consistency. JSON marshaling does not validate.

## Typed errors

Use `errors.Is` for `rootfsdiff.ErrInvalidInput`, `ErrUnsupportedInput` and `ErrLimitExceeded`. Use `errors.As` for detailed `*rootfsdiff.Error` operation/archive-path context and `*rootfsdiff.LimitError` budget fields. Underlying I/O and context error identities are preserved through wrapping.

```go
var limit *rootfsdiff.LimitError
var input *rootfsdiff.Error
switch {
case errors.As(err, &limit):
    fmt.Fprintf(os.Stderr, "%s exceeded: observed %d, maximum %d\n",
        limit.Limit, limit.Actual, limit.Maximum)
case errors.Is(err, rootfsdiff.ErrUnsupportedInput):
    fmt.Fprintln(os.Stderr, "unsupported archive feature:", err)
case errors.As(err, &input):
    fmt.Fprintf(os.Stderr, "%s at archive path %q: %v\n", input.Op, input.Path, input.Err)
case errors.Is(err, context.Canceled):
    fmt.Fprintln(os.Stderr, "comparison canceled")
default:
    fmt.Fprintln(os.Stderr, err)
}
```

This snippet illustrates classification inside an error branch. Test cancellation or other underlying sentinels independently if they take precedence in your application; an I/O/cancellation error may also carry `*Error` context. `LimitError.Actual` is the first observed value beyond `Maximum`, not necessarily the size of unread input. Never use a manifest returned with a scan error as a successful partial result.

## Compatibility

The module is in 0.x development; pin an audited commit/resolved module version and review API changes when updating. JSON has its own explicit `schema_version`, currently 1. Check it when consuming reports. Schema changes require coordinated model, fixture and consumer updates. See [model fields](models.md), [report output](reports.md) and the [semantic contract](v0.1-contract.md).
