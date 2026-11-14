package rootfsdiff

import (
	"cmp"
	"math"
	"slices"
	"strings"
)

// Compare returns the complete deterministic difference between two manifests.
// It validates both complete manifests and opts before applying ignore rules.
// Entries may arrive in any order. Inputs are never mutated, and returned change
// sides are independent copies; callers must not mutate inputs during a call.
//
// Exact ignores match only their literal canonical path. A prefix matches itself
// and descendants separated by '/', so "var/log" does not match "var/logger".
// Both sides are filtered alike. Every warning survives filtering and receives
// its before/after source. No implicit directory entries are manufactured.
//
// Changes sort by path and list every differing field in this order: kind, mode,
// uid, gid, size, sha256, link_target, device_major, device_minor. Type changes
// are single modified records. Only Entry fields are compared: timestamps,
// user/group names, directory sizes and excluded metadata are not represented
// as comparable fields. Hardlinks use their validated effective inode metadata.
//
// Payload totals sum retained regular-entry sizes with checked int64 arithmetic;
// hardlink aliases and all other kinds contribute zero. Net growth is after
// minus before, including additions, removals and type transitions. This is
// uncompressed regular payload, not flash size or filesystem disk usage.
// PositiveGrowth contains every positive per-path regular-payload delta, sorted
// by descending bytes then path, without a presentation limit. Failures return
// a zero Diff and a typed validation error, never a successful partial result.
func Compare(before, after Manifest, opts CompareOptions) (Diff, error) {
	if err := before.Validate(); err != nil {
		return Diff{}, &Error{Op: "compare before", Err: err}
	}
	if err := after.Validate(); err != nil {
		return Diff{}, &Error{Op: "compare after", Err: err}
	}
	if err := opts.Validate(); err != nil {
		return Diff{}, &Error{Op: "compare options", Err: err}
	}

	ignore := compareIgnores{exact: make(map[string]bool), prefixes: make(map[string]bool)}
	for _, p := range opts.IgnorePaths {
		ignore.exact[p] = true
	}
	for _, p := range opts.IgnorePrefixes {
		ignore.prefixes[p] = true
	}
	b, beforeBytes, err := compareRetained(before.Entries, ignore)
	if err != nil {
		return Diff{}, err
	}
	a, afterBytes, err := compareRetained(after.Entries, ignore)
	if err != nil {
		return Diff{}, err
	}
	d := Diff{
		Summary: Summary{
			BeforeEntries: int64(len(b)), AfterEntries: int64(len(a)),
			BeforePayloadBytes: beforeBytes, AfterPayloadBytes: afterBytes,
			// Each total is in [0, MaxInt64], so subtraction cannot overflow.
			NetGrowthBytes: afterBytes - beforeBytes,
		},
		Changes: []Change{}, PositiveGrowth: []Growth{}, Warnings: []Warning{},
	}
	for i, j := 0, 0; i < len(b) || j < len(a); {
		var c Change
		switch {
		case j == len(a) || (i < len(b) && b[i].Path < a[j].Path):
			e := b[i]
			c = Change{Path: e.Path, Kind: ChangeRemoved, Before: &e, ChangedFields: []Field{}}
			d.Summary.Removed++
			i++
		case i == len(b) || a[j].Path < b[i].Path:
			e := a[j]
			c = Change{Path: e.Path, Kind: ChangeAdded, After: &e, ChangedFields: []Field{}}
			d.Summary.Added++
			j++
		default:
			old, next := b[i], a[j]
			i++
			j++
			fields := changedFields(old, next)
			if len(fields) == 0 {
				d.Summary.Unchanged++
				continue
			}
			c = Change{Path: old.Path, Kind: ChangeModified, Before: &old, After: &next, ChangedFields: fields}
			d.Summary.Modified++
		}
		d.Changes = append(d.Changes, c)
		oldBytes, nextBytes := comparePayload(c.Before), comparePayload(c.After)
		if nextBytes > oldBytes {
			d.PositiveGrowth = append(d.PositiveGrowth, Growth{Path: c.Path, Bytes: nextBytes - oldBytes})
		}
	}
	slices.SortFunc(d.PositiveGrowth, func(a, b Growth) int {
		if c := cmp.Compare(b.Bytes, a.Bytes); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	for _, side := range []struct {
		manifest Manifest
		source   WarningSource
	}{{before, SourceBefore}, {after, SourceAfter}} {
		for _, w := range side.manifest.Warnings {
			w.Source = side.source
			d.Warnings = append(d.Warnings, w)
		}
	}
	slices.SortFunc(d.Warnings, compareWarning)
	return d, nil
}

type compareIgnores struct {
	exact    map[string]bool
	prefixes map[string]bool
}

func (rules compareIgnores) matches(path string) bool {
	if rules.exact[path] {
		return true
	}
	for p := path; ; {
		if rules.prefixes[p] {
			return true
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			return false
		}
		p = p[:i]
	}
}

func compareRetained(entries []Entry, ignore compareIgnores) ([]Entry, int64, error) {
	retained := make([]Entry, 0, len(entries))
	var total int64
	for _, e := range entries {
		if ignore.matches(e.Path) {
			continue
		}
		if e.Kind == KindRegular {
			if e.Size > math.MaxInt64-total {
				return nil, 0, invalid("compare payload", e.Path, "regular payload total overflows int64")
			}
			total += e.Size
		}
		retained = append(retained, e)
	}
	slices.SortFunc(retained, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	return retained, total, nil
}

func comparePayload(e *Entry) int64 {
	if e != nil && e.Kind == KindRegular {
		return e.Size
	}
	return 0
}
