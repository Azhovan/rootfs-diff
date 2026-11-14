// Package rootfsdiff compares recorded contents and metadata of root filesystem
// tar archives without extracting or executing their contents.
//
// Scan reads tar/tar.gz streams, Compare produces deterministic changes, and
// ParsePolicy/Evaluate apply explicit required-path and payload-growth gates.
// The separate public report package renders text, JSON and Markdown; the CLI
// composes these operations. See docs/v0.1-contract.md for their semantics.
//
// Call Validate on caller-built models or after decoding model JSON. Validation
// does not normalize, sort, mutate, print or access the filesystem. Manifests
// may have arbitrary input order; produced manifests and diffs are sorted.
// Complete manifests validate hardlink closure. Individual change entries need
// not include their target because comparison ignores can remove that target.
// Nil model slices marshal as empty arrays, and absent change sides as null.
// Warning path "." preserves metadata warnings on an ignored root directory
// record without retaining or synthesizing a root entry.
//
// Core functions never extract or execute archive contents. Scan
// accepts a caller-owned io.Reader and does not close it. Context cancellation
// is checked between bounded reads; it cannot interrupt an arbitrary reader
// already blocked inside Read. Independent calls have no mutable global options;
// callers must not concurrently mutate input models while a call reads them.
// Sizes, counters and numeric ownership use int64; JSON modes are decimal 07777
// bit masks, while human renderers use octal. See the contract for exclusions,
// limits, error categories and the complete schema.
package rootfsdiff
