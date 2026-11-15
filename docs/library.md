# Go library

Import `github.com/Azhovan/rootfs-diff` as package `rootfsdiff` to scan archives, compare their contents and evaluate policies. This package also defines the shared data models. Import `github.com/Azhovan/rootfs-diff/report` to write a report. The root package works independently of report formatting and command-line behavior.

The [runnable library example](../examples/library/README.md) shows the complete process, from scanning archives to writing a report. See [module setup](installation.md#use-the-go-module) to add the dependency to your own module.

## Operations

```go
Scan(context.Context, io.Reader, ScanOptions) (Manifest, error)
Compare(before, after Manifest, opts CompareOptions) (Diff, error)
ParsePolicy(io.Reader) (Policy, error)
Evaluate(after Manifest, diff Diff, policy Policy) ([]Violation, error)
```

`Scan` reads one tar or tar.gz stream and returns a manifest: the entries recorded in the archive, with normalized paths and any metadata warnings. Use `ScanOptions{}` for the default limits, or set individual fields to positive values to override them.

`Compare` validates both manifests and returns their differences. It applies `CompareOptions{IgnorePaths: ..., IgnorePrefixes: ...}` to both sides. `Evaluate` checks required paths against the complete candidate manifest and checks net growth using the diff, where ignores have already been applied. Pass the candidate manifest and diff from the same comparison. The diff does not contain enough information to reconstruct an ignored baseline.

Use `ParsePolicy` to read untrusted policy input. It enforces the 1 MiB size limit and rejects duplicate keys, unknown keys, missing required fields and trailing input. Decoding with `encoding/json` and then calling `Policy.Validate` does not perform all of these checks.

`Policy.MaxTotalGrowthBytes` has type `*int64`. A nil pointer disables the growth check; a pointer to zero prohibits positive net growth.

The public renderer's signature is:

```go
report.Write(io.Writer, rootfsdiff.Report, report.Format) error
```

Choose `report.Text`, `report.JSON` or `report.Markdown`. A zero or unknown format returns an unsupported-input error. Assemble the report from the diff and violations returned by the library:

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

`MetadataExclusions()` returns a fresh slice containing the complete list of metadata that is not compared. `Report.Validate` requires that list in every report.

The renderer validates the report, encodes it in memory and writes it once. It does not retain archive file contents. Check the returned error: a writer can fail after accepting part of the report. Policy violations are returned as data. Your application decides whether violations or `Diff.Changes` should cause it to exit with an error.

## Ownership, validation and concurrency

Your application owns its readers and writers. `Scan` and `ParsePolicy` do not close their readers; `report.Write` does not close its writer. Open and close files in your application, and check both operation errors and any relevant close errors. Scanning consumes file contents without retaining or extracting them. To scan again, supply a fresh reader or rewind one that supports seeking.

`Scan` reads in bounded chunks and checks for context cancellation between reads. It cannot interrupt a `Read` that is already blocked; the reader must provide its own way to unblock that operation. `Compare`, `ParsePolicy`, `Evaluate` and `report.Write` do not accept a context. Their work depends on the input limits and the behavior of the readers and writers you supply.

Library operations do not change your models or options, print output, exit the process or use mutable global options. You can run independent calls concurrently if they use separate readers and writers and you do not change their input models at the same time. The slices and pointers in model structs are ordinary mutable Go values; the API does not synchronize your writes. Do not share an advancing input stream or write concurrently to the same destination without your own synchronization.

If you build a manifest yourself, its entries and warnings can be in any order, but its paths must already be canonical. `ValidatePath` checks path spelling without normalizing it, accessing the host filesystem or applying scan length limits. Manifest validation checks that fields are appropriate for each entry kind, paths are unique, recorded ancestors are directories, hardlink chains resolve to regular entries with matching effective metadata, and totals do not overflow.

Manifests and diffs produced by the library are sorted. Validation of diffs and reports also checks the required output order and consistency between fields. Marshaling a value to JSON does not validate it.

## Typed errors

Use `errors.Is` to check for `rootfsdiff.ErrInvalidInput`, `ErrUnsupportedInput` or `ErrLimitExceeded`. Use `errors.As` to inspect the operation and archive path in a `*rootfsdiff.Error`, or the limit details in a `*rootfsdiff.LimitError`. Wrapped I/O and context errors remain available through `errors.Is` and `errors.As`.

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

Use this snippet after an operation returns a non-nil error. An I/O or cancellation error can also be wrapped in `*Error`. If your application needs to handle cancellation first, move that case before the `errors.As(err, &input)` case.

`LimitError.Actual` is the first observed value above `Maximum`; it may be smaller than the eventual total if input remains unread. When `Scan` returns an error, do not use the returned manifest as a successful partial result.

## Compatibility

The module is in 0.x development. Pin a commit or resolved module version you have reviewed, and check for API changes when updating it. JSON reports have a separate `schema_version`, currently 1; check this field before consuming a report. Schema changes require corresponding updates to models, fixtures and consumers. See [model fields](models.md), [report output](reports.md) and the [semantic contract](v0.1-contract.md).
