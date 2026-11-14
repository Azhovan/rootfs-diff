// Package rootfsdiff defines the shared models and validation contract for
// comparing recorded contents of root filesystem tar archives.
//
// Foundation currently implements models, validation, options and error types.
// Scanner, comparison and policy entry points are specified in docs/v0.1-contract.md
// and arrive in their owning implementation issues. There are no success stubs.
// The public report package and CLI likewise arrive in later issues.
//
// Call Validate on caller-built models or after decoding model JSON. Validation
// does not normalize, sort, mutate, print or access the filesystem. Manifests
// may have arbitrary input order; produced manifests and diffs will be sorted.
// Complete manifests validate hardlink closure. Individual change entries need
// not include their target because comparison ignores can remove that target.
// Nil model slices marshal as empty arrays, and absent change sides as null.
// Warning path "." preserves metadata warnings on an ignored root directory
// record without retaining or synthesizing a root entry.
//
// Core functions never extract or execute archive contents. The planned Scan
// accepts a caller-owned io.Reader and does not close it. Context cancellation
// is checked between bounded reads; it cannot interrupt an arbitrary reader
// already blocked inside Read. Independent calls have no mutable global options;
// callers must not concurrently mutate input models while a call reads them.
// Sizes, counters and numeric ownership use int64; JSON modes are decimal 07777
// bit masks, while human renderers use octal. See the contract for exclusions,
// limits, error categories and the complete schema.
package rootfsdiff
