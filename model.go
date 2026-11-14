package rootfsdiff

import (
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

const SchemaVersion = 1

type EntryKind string

const (
	KindRegular     EntryKind = "regular"
	KindDirectory   EntryKind = "directory"
	KindSymlink     EntryKind = "symlink"
	KindHardlink    EntryKind = "hardlink"
	KindCharDevice  EntryKind = "char_device"
	KindBlockDevice EntryKind = "block_device"
	KindFIFO        EntryKind = "fifo"
)

// Entry records explicit archive metadata, with numeric mode bits (07777 mask)
// and lowercase hexadecimal SHA-256. Inapplicable fields are zero or empty.
// Hardlinks retain their own path/kind/target and use terminal regular-entry
// mode, ownership, size and digest. UID/GID preserve signed numeric tar IDs.
type Entry struct {
	Path        string    `json:"path"`
	Kind        EntryKind `json:"kind"`
	Mode        uint32    `json:"mode"`
	UID         int64     `json:"uid"`
	GID         int64     `json:"gid"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	LinkTarget  string    `json:"link_target"`
	DeviceMajor int64     `json:"device_major"`
	DeviceMinor int64     `json:"device_minor"`
}

// ValidatePath checks a canonical POSIX root-relative model path. It does not
// normalize input, impose scan limits, interpret backslashes or glob syntax,
// or consult the host filesystem. Root records are absent from manifests.
func ValidatePath(p string) error {
	if p == "" || !utf8.ValidString(p) || strings.HasPrefix(p, "/") {
		return invalid("validate path", p, "expected a nonempty UTF-8 root-relative path")
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return invalid("validate path", p, "path must be canonical, without empty, '.' or '..' segments")
		}
	}
	return nil
}

// Validate checks one entry. Manifest.Validate additionally checks ancestors,
// hardlink closure/effective data, uniqueness and aggregate size overflow.
func (e Entry) Validate() error {
	if err := ValidatePath(e.Path); err != nil {
		return err
	}
	if e.Mode > 07777 {
		return invalid("validate entry", e.Path, "mode contains bits outside 07777")
	}
	if e.Size < 0 || e.DeviceMajor < 0 || e.DeviceMinor < 0 {
		return invalid("validate entry", e.Path, "size and device numbers must be nonnegative int64 values")
	}
	switch e.Kind {
	case KindRegular, KindHardlink:
		decoded, err := hex.DecodeString(e.SHA256)
		if err != nil || len(decoded) != 32 || e.SHA256 != strings.ToLower(e.SHA256) {
			return invalid("validate entry", e.Path, "regular effective data requires a lowercase 64-digit SHA-256")
		}
	case KindDirectory, KindSymlink, KindCharDevice, KindBlockDevice, KindFIFO:
		if e.Size != 0 || e.SHA256 != "" {
			return invalid("validate entry", e.Path, "non-file entries must have zero size and empty SHA-256")
		}
	default:
		return invalid("validate entry", e.Path, "unknown entry kind")
	}
	if e.Kind != KindCharDevice && e.Kind != KindBlockDevice && (e.DeviceMajor != 0 || e.DeviceMinor != 0) {
		return invalid("validate entry", e.Path, "device numbers apply only to devices")
	}
	switch e.Kind {
	case KindHardlink:
		if err := ValidatePath(e.LinkTarget); err != nil {
			return invalid("validate entry", e.Path, "hardlink target must be a canonical path")
		}
	case KindSymlink:
		if !utf8.ValidString(e.LinkTarget) {
			return invalid("validate entry", e.Path, "symlink target must be UTF-8")
		}
	default:
		if e.LinkTarget != "" {
			return invalid("validate entry", e.Path, "link target applies only to links")
		}
	}
	return nil
}

type WarningCode string
type WarningSource string

const (
	WarningUncomparedMetadata WarningCode   = "uncompared_metadata"
	SourceBefore              WarningSource = "before"
	SourceAfter               WarningSource = "after"
)

// Warning identifies a metadata key, never its value. Manifest warnings have
// empty Source; comparison/report warnings have a before or after Source.
// Path is canonical, except "." identifies an ignored root directory record.
type Warning struct {
	Source WarningSource `json:"source"`
	Path   string        `json:"path"`
	Code   WarningCode   `json:"code"`
	Key    string        `json:"key"`
}

func (w Warning) Validate() error {
	if w.Path != "." {
		if err := ValidatePath(w.Path); err != nil {
			return err
		}
	}
	if w.Code != WarningUncomparedMetadata || w.Key == "" || !utf8.ValidString(w.Key) {
		return invalid("validate warning", w.Path, "expected uncompared_metadata with a nonempty UTF-8 key")
	}
	if w.Source != "" && w.Source != SourceBefore && w.Source != SourceAfter {
		return invalid("validate warning", w.Path, "unknown warning source")
	}
	return nil
}

// Manifest contains only explicit entries. Caller-built entries and warnings
// may have any order; producers return sorted slices. Nil means empty.
type Manifest struct {
	Entries  []Entry   `json:"entries"`
	Warnings []Warning `json:"warnings"`
}

func (m Manifest) Validate() error {
	entries := make(map[string]Entry, len(m.Entries))
	var total int64
	for _, e := range m.Entries {
		if err := e.Validate(); err != nil {
			return err
		}
		if _, exists := entries[e.Path]; exists {
			return invalid("validate manifest", e.Path, "entries must have unique paths")
		}
		entries[e.Path] = e
		if e.Kind == KindRegular {
			if e.Size > math.MaxInt64-total {
				return invalid("validate manifest", e.Path, "regular payload total overflows int64")
			}
			total += e.Size
		}
	}
	for _, e := range m.Entries {
		for p, _, ok := strings.Cut(e.Path, "/"); ok; {
			if ancestor, exists := entries[p]; exists && ancestor.Kind != KindDirectory {
				return invalid("validate manifest", e.Path, "explicit ancestor "+fmt.Sprintf("%q", p)+" is not a directory")
			}
			rest := e.Path[len(p)+1:]
			next, _, more := strings.Cut(rest, "/")
			p, ok = p+"/"+next, more
		}
	}
	// Cache terminal regular paths so even long forward chains take linear work.
	resolved := make(map[string]string)
	for _, e := range m.Entries {
		if e.Kind != KindHardlink {
			continue
		}
		chain := []string{}
		seen := make(map[string]bool)
		p := e.Path
		var terminal Entry
		for {
			if target, ok := resolved[p]; ok {
				terminal = entries[target]
				break
			}
			current, ok := entries[p]
			if !ok {
				return invalid("validate manifest", e.Path, "missing hardlink target "+fmt.Sprintf("%q", p))
			}
			if current.Kind == KindRegular {
				terminal = current
				break
			}
			if current.Kind != KindHardlink {
				return invalid("validate manifest", e.Path, "hardlink does not end in a regular entry")
			}
			if seen[p] {
				return invalid("validate manifest", e.Path, "hardlink cycle")
			}
			seen[p] = true
			chain = append(chain, p)
			p = current.LinkTarget
		}
		for _, alias := range chain {
			a := entries[alias]
			if a.Mode != terminal.Mode || a.UID != terminal.UID || a.GID != terminal.GID || a.Size != terminal.Size || a.SHA256 != terminal.SHA256 {
				return invalid("validate manifest", alias, "hardlink effective data differs from terminal regular entry")
			}
			resolved[alias] = terminal.Path
		}
	}
	seenWarnings := make(map[Warning]bool, len(m.Warnings))
	for _, w := range m.Warnings {
		if err := w.Validate(); err != nil {
			return err
		}
		if w.Source != "" || seenWarnings[w] {
			return invalid("validate manifest", w.Path, "warnings must be unique and have no source")
		}
		seenWarnings[w] = true
		if _, ok := entries[w.Path]; !ok && w.Path != "." {
			return invalid("validate manifest", w.Path, "warning must identify an explicit entry or ignored root directory")
		}
	}
	return nil
}

type ChangeKind string
type Field string

const (
	ChangeAdded      ChangeKind = "added"
	ChangeRemoved    ChangeKind = "removed"
	ChangeModified   ChangeKind = "modified"
	FieldKind        Field      = "kind"
	FieldMode        Field      = "mode"
	FieldUID         Field      = "uid"
	FieldGID         Field      = "gid"
	FieldSize        Field      = "size"
	FieldSHA256      Field      = "sha256"
	FieldLinkTarget  Field      = "link_target"
	FieldDeviceMajor Field      = "device_major"
	FieldDeviceMinor Field      = "device_minor"
)

// Change has exactly the applicable sides. Modified fields are complete and in
// the fixed contract order. Added/removed records use an empty field array.
type Change struct {
	Path          string     `json:"path"`
	Kind          ChangeKind `json:"kind"`
	Before        *Entry     `json:"before"`
	After         *Entry     `json:"after"`
	ChangedFields []Field    `json:"changed_fields"`
}

func (c Change) Validate() error {
	if err := ValidatePath(c.Path); err != nil {
		return err
	}
	for _, e := range []*Entry{c.Before, c.After} {
		if e != nil {
			if err := e.Validate(); err != nil {
				return err
			}
			if e.Path != c.Path {
				return invalid("validate change", c.Path, "side path differs from change path")
			}
		}
	}
	switch c.Kind {
	case ChangeAdded:
		if c.Before != nil || c.After == nil || len(c.ChangedFields) != 0 {
			return invalid("validate change", c.Path, "added change requires only after and no changed fields")
		}
	case ChangeRemoved:
		if c.Before == nil || c.After != nil || len(c.ChangedFields) != 0 {
			return invalid("validate change", c.Path, "removed change requires only before and no changed fields")
		}
	case ChangeModified:
		if c.Before == nil || c.After == nil {
			return invalid("validate change", c.Path, "modified change requires both sides")
		}
		want := changedFields(*c.Before, *c.After)
		if len(want) == 0 || !slices.Equal(c.ChangedFields, want) {
			return invalid("validate change", c.Path, "changed fields must list every difference in contract order")
		}
	default:
		return invalid("validate change", c.Path, "unknown change kind")
	}
	return nil
}

func changedFields(a, b Entry) []Field {
	fields := []struct {
		field Field
		diff  bool
	}{
		{FieldKind, a.Kind != b.Kind}, {FieldMode, a.Mode != b.Mode},
		{FieldUID, a.UID != b.UID}, {FieldGID, a.GID != b.GID},
		{FieldSize, a.Size != b.Size}, {FieldSHA256, a.SHA256 != b.SHA256},
		{FieldLinkTarget, a.LinkTarget != b.LinkTarget},
		{FieldDeviceMajor, a.DeviceMajor != b.DeviceMajor}, {FieldDeviceMinor, a.DeviceMinor != b.DeviceMinor},
	}
	var result []Field
	for _, f := range fields {
		if f.diff {
			result = append(result, f.field)
		}
	}
	return result
}

// Summary counts entries after ignore filtering, not inode identities. Payload
// totals count regular entries only. All counts and byte values are int64.
type Summary struct {
	BeforeEntries      int64 `json:"before_entries"`
	AfterEntries       int64 `json:"after_entries"`
	Added              int64 `json:"added"`
	Removed            int64 `json:"removed"`
	Modified           int64 `json:"modified"`
	Unchanged          int64 `json:"unchanged"`
	BeforePayloadBytes int64 `json:"before_payload_bytes"`
	AfterPayloadBytes  int64 `json:"after_payload_bytes"`
	NetGrowthBytes     int64 `json:"net_growth_bytes"`
}

func (s Summary) Validate() error {
	values := []int64{s.BeforeEntries, s.AfterEntries, s.Added, s.Removed, s.Modified, s.Unchanged, s.BeforePayloadBytes, s.AfterPayloadBytes}
	for _, n := range values {
		if n < 0 {
			return invalid("validate summary", "", "counts and payload totals must be nonnegative")
		}
	}
	if s.Modified > s.BeforeEntries || s.Removed > s.BeforeEntries-s.Modified || s.Unchanged != s.BeforeEntries-s.Modified-s.Removed ||
		s.Modified > s.AfterEntries || s.Added > s.AfterEntries-s.Modified || s.Unchanged != s.AfterEntries-s.Modified-s.Added {
		return invalid("validate summary", "", "entry counts are inconsistent")
	}
	if s.NetGrowthBytes != s.AfterPayloadBytes-s.BeforePayloadBytes {
		return invalid("validate summary", "", "net growth differs from payload totals")
	}
	if (s.BeforeEntries == 0 && s.BeforePayloadBytes != 0) || (s.AfterEntries == 0 && s.AfterPayloadBytes != 0) {
		return invalid("validate summary", "", "empty sides cannot contain payload bytes")
	}
	return nil
}

type Growth struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (g Growth) Validate() error {
	if err := ValidatePath(g.Path); err != nil {
		return err
	}
	if g.Bytes <= 0 {
		return invalid("validate growth", g.Path, "growth bytes must be positive")
	}
	return nil
}

// Diff records every retained change and every warning, even for ignored paths.
// PositiveGrowth is complete, ordered by decreasing bytes then increasing path.
type Diff struct {
	Summary        Summary   `json:"summary"`
	Changes        []Change  `json:"changes"`
	PositiveGrowth []Growth  `json:"positive_growth"`
	Warnings       []Warning `json:"warnings"`
}

func (d Diff) Validate() error {
	if err := d.Summary.Validate(); err != nil {
		return err
	}
	var added, removed, modified, beforeBytes, afterBytes int64
	expectedGrowth := make(map[string]int64)
	for i, c := range d.Changes {
		if err := c.Validate(); err != nil {
			return err
		}
		if i > 0 && d.Changes[i-1].Path >= c.Path {
			return invalid("validate diff", c.Path, "changes must be unique and sorted by path")
		}
		switch c.Kind {
		case ChangeAdded:
			added++
		case ChangeRemoved:
			removed++
		case ChangeModified:
			modified++
		}
		var b, a int64
		if c.Before != nil && c.Before.Kind == KindRegular {
			b = c.Before.Size
		}
		if c.After != nil && c.After.Kind == KindRegular {
			a = c.After.Size
		}
		if b > math.MaxInt64-beforeBytes || a > math.MaxInt64-afterBytes {
			return invalid("validate diff", c.Path, "changed payload totals overflow int64")
		}
		beforeBytes += b
		afterBytes += a
		if a > b {
			expectedGrowth[c.Path] = a - b
		}
	}
	s := d.Summary
	if added != s.Added || removed != s.Removed || modified != s.Modified || beforeBytes > s.BeforePayloadBytes || afterBytes > s.AfterPayloadBytes ||
		s.BeforePayloadBytes-beforeBytes != s.AfterPayloadBytes-afterBytes || (s.Unchanged == 0 && s.BeforePayloadBytes != beforeBytes) {
		return invalid("validate diff", "", "summary disagrees with changes or unchanged payload bytes")
	}
	if len(d.PositiveGrowth) != len(expectedGrowth) {
		return invalid("validate diff", "", "positive growth must include exactly the growing regular paths")
	}
	for i, g := range d.PositiveGrowth {
		if err := g.Validate(); err != nil {
			return err
		}
		if i > 0 {
			prev := d.PositiveGrowth[i-1]
			if prev.Bytes < g.Bytes || (prev.Bytes == g.Bytes && prev.Path >= g.Path) {
				return invalid("validate diff", g.Path, "growth must sort by descending bytes then unique path")
			}
		}
		if expectedGrowth[g.Path] != g.Bytes {
			return invalid("validate diff", g.Path, "growth disagrees with change")
		}
		delete(expectedGrowth, g.Path)
	}
	return validateWarnings(d.Warnings, true)
}

type Policy struct {
	SchemaVersion       int      `json:"schema_version"`
	RequiredPaths       []string `json:"required_paths"`
	MaxTotalGrowthBytes *int64   `json:"max_total_growth_bytes,omitempty"`
}

func (p Policy) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return invalid("validate policy", "", "unsupported schema version")
	}
	for _, path := range p.RequiredPaths {
		if err := ValidatePath(path); err != nil {
			return err
		}
	}
	if p.MaxTotalGrowthBytes != nil && *p.MaxTotalGrowthBytes < 0 {
		return invalid("validate policy", "", "maximum total growth must be nonnegative")
	}
	return nil
}

type PolicyRule string

const (
	RuleRequiredPath        PolicyRule = "required_path"
	RuleMaxTotalGrowthBytes PolicyRule = "max_total_growth_bytes"
)

// Violation has a path only for required_path and numeric values only for the
// growth rule. Renderers generate descriptions from these stable fields.
type Violation struct {
	Rule   PolicyRule `json:"rule"`
	Path   string     `json:"path"`
	Limit  *int64     `json:"limit"`
	Actual *int64     `json:"actual"`
}

func (v Violation) Validate() error {
	switch v.Rule {
	case RuleRequiredPath:
		if err := ValidatePath(v.Path); err != nil {
			return err
		}
		if v.Limit != nil || v.Actual != nil {
			return invalid("validate violation", v.Path, "required-path violation has no numeric values")
		}
	case RuleMaxTotalGrowthBytes:
		if v.Path != "" || v.Limit == nil || v.Actual == nil || *v.Limit < 0 || *v.Actual <= *v.Limit {
			return invalid("validate violation", v.Path, "growth violation requires empty path and actual greater than nonnegative limit")
		}
	default:
		return invalid("validate violation", v.Path, "unknown policy rule")
	}
	return nil
}

// MetadataExclusions returns a fresh, fixed-order slice for every report.
func MetadataExclusions() []string {
	return []string{"acls", "directory_sizes", "file_capabilities", "selinux_labels", "timestamps", "unrecognized_pax_metadata", "user_group_names", "xattrs"}
}

// Report is the complete schema-version-1 output. No filenames, timestamps or
// run-dependent values belong here. A renderer must validate before writing.
type Report struct {
	SchemaVersion      int         `json:"schema_version"`
	Summary            Summary     `json:"summary"`
	Changes            []Change    `json:"changes"`
	PositiveGrowth     []Growth    `json:"positive_growth"`
	Violations         []Violation `json:"violations"`
	Warnings           []Warning   `json:"warnings"`
	MetadataExclusions []string    `json:"metadata_exclusions"`
}

func (r Report) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return invalid("validate report", "", "unsupported schema version")
	}
	if err := (Diff{Summary: r.Summary, Changes: r.Changes, PositiveGrowth: r.PositiveGrowth, Warnings: r.Warnings}).Validate(); err != nil {
		return err
	}
	for i, v := range r.Violations {
		if err := v.Validate(); err != nil {
			return err
		}
		if i > 0 {
			prev := r.Violations[i-1]
			if prev.Rule > v.Rule || (prev.Rule == v.Rule && prev.Path >= v.Path) {
				return invalid("validate report", v.Path, "violations must be unique and sorted by rule then path")
			}
		}
		if v.Rule == RuleMaxTotalGrowthBytes && *v.Actual != r.Summary.NetGrowthBytes {
			return invalid("validate report", "", "growth violation actual differs from summary")
		}
	}
	if !slices.Equal(r.MetadataExclusions, MetadataExclusions()) {
		return invalid("validate report", "", "metadata exclusions must be the complete standard list in fixed order")
	}
	return nil
}

func validateWarnings(warnings []Warning, sourced bool) error {
	for i, w := range warnings {
		if err := w.Validate(); err != nil {
			return err
		}
		if (sourced && w.Source == "") || (!sourced && w.Source != "") {
			return invalid("validate warnings", w.Path, "source must be absent in manifests and present in diffs/reports")
		}
		if i > 0 && compareWarning(warnings[i-1], w) >= 0 {
			return invalid("validate warnings", w.Path, "warnings must be unique and sorted by path, code, key, source")
		}
	}
	return nil
}

func compareWarning(a, b Warning) int {
	for _, pair := range [][2]string{{a.Path, b.Path}, {string(a.Code), string(b.Code)}, {a.Key, b.Key}, {string(a.Source), string(b.Source)}} {
		if c := strings.Compare(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	return 0
}
