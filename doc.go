// Package rootfsdiff compares recorded contents and metadata of root filesystem
// tar archives without extracting or executing their contents.
//
// Scan reads tar and tar.gz streams into manifests. Compare reports changes
// between a baseline and candidate manifest. ParsePolicy and Evaluate apply
// user-defined checks for required paths and net payload growth. The separate
// public report package writes text, JSON and Markdown reports; the CLI calls
// these operations in sequence. See docs/v0.1-contract.md for the exact behavior.
//
// Call Validate on caller-built models or after decoding model JSON. Validation
// does not normalize, sort or change the input, print output, or access the
// filesystem. Caller-built manifests may list entries in any order; the scanner
// and comparator sort their output. Manifest validation checks that hardlink
// chains end at existing regular files and have matching effective metadata.
// A diff need not contain the entry named by a hardlink target, since ignore
// rules may have removed that entry from the comparison.
// Nil model slices encode as empty JSON arrays, and absent change sides as null.
// A warning may use path "." for metadata on an ignored root directory record,
// even though the manifest has no root entry.
//
// Core functions never extract or execute archive contents. Scan accepts an
// io.Reader that the caller owns and is responsible for closing. Cancellation
// is checked between bounded reads; it cannot interrupt an arbitrary reader
// already blocked inside Read. Independent calls share no mutable global
// options. Callers must not modify input models while a call is reading them.
// Sizes, counters and numeric ownership use int64. JSON modes are decimal
// integers containing the permission and special bits in the 07777 mask;
// text and Markdown reports show those modes in octal. The contract describes
// metadata exclusions, limits, error categories and the complete schema.
package rootfsdiff
