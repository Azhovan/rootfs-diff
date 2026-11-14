// Package report renders validated rootfsdiff reports without scanning archives
// or evaluating policies. See the package README for output and error semantics.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	rd "github.com/Azhovan/rootfs-diff"
)

// Format selects the report representation. The zero value is unsupported.
type Format string

const (
	Text     Format = "text"
	JSON     Format = "json"
	Markdown Format = "markdown"
)

// Write validates r and writes a deterministic report in format. Invalid models,
// unsupported formats and a nil writer produce no output. Inputs are not mutated.
//
// JSON preserves every schema-version-1 field, including all positive growth.
// Text and Markdown show at most ten growth rankings and every change, with full
// applicable entry metadata and octal modes. All human-facing untrusted strings
// are quoted and escaped. No format asserts firmware security or equivalence.
//
// A writer failure may leave a partial report. Write preserves underlying errors
// for errors.Is/errors.As and returns io.ErrShortWrite for a short, nil-error
// write. It does not retry, close or flush the caller's writer. Concurrent calls
// are safe with independent writers; concurrent mutation of r is unsupported.
func Write(w io.Writer, r rd.Report, format Format) error {
	if format != Text && format != JSON && format != Markdown {
		return &rd.Error{Op: "write report", Kind: rd.ErrorUnsupportedInput,
			Err: fmt.Errorf("unsupported format %q", format)}
	}
	if w == nil {
		return &rd.Error{Op: "write report", Kind: rd.ErrorInvalidInput, Err: errors.New("nil writer")}
	}
	if err := r.Validate(); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	var data []byte
	if format == JSON {
		var err error
		data, err = json.MarshalIndent(r, "", "  ")
		if err != nil {
			return fmt.Errorf("encode report: %w", err)
		}
		data = append(jsonControls(data), '\n')
	} else {
		data = []byte(human(r, format == Markdown))
	}
	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write report: %w", io.ErrShortWrite)
	}
	return nil
}

// JSON permits raw C1 and Unicode format controls. Escape those too so viewing
// a JSON artifact in a terminal cannot activate controls hidden in string data.
func jsonControls(data []byte) []byte {
	var b strings.Builder
	for _, r := range string(data) {
		if r >= 0x7f && !strconv.IsPrint(r) {
			if r <= 0xffff {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				hi, lo := utf16.EncodeRune(r)
				fmt.Fprintf(&b, "\\u%04x\\u%04x", hi, lo)
			}
		} else {
			b.WriteRune(r)
		}
	}
	return []byte(b.String())
}

// literal makes controls (including Unicode format/bidi controls) visible. In
// Markdown, punctuation becomes entities inside an inline HTML code element:
// entities cannot introduce Markdown syntax or open HTML tags when rendered.
func literal(s string, markdown bool) string {
	quoted := strconv.Quote(s)
	if !markdown {
		return quoted
	}
	var b strings.Builder
	b.WriteString("<code>")
	for _, r := range quoted {
		if r < 128 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ') {
			fmt.Fprintf(&b, "&#%d;", r)
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteString("</code>")
	return b.String()
}

func human(r rd.Report, markdown bool) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	heading := func(title string) {
		if markdown {
			line("## %s\n", title)
		} else {
			line("%s:", title)
		}
	}
	item := "  "
	if markdown {
		item = "- "
		line("# rootfs-diff report (schema %d)\n", r.SchemaVersion)
	} else {
		line("rootfs-diff report (schema %d)\n", r.SchemaVersion)
	}
	s := r.Summary
	heading("Summary")
	line(item+"Entries: before %d, after %d", s.BeforeEntries, s.AfterEntries)
	line(item+"Changes: added %d, removed %d, modified %d, unchanged %d", s.Added, s.Removed, s.Modified, s.Unchanged)
	line(item+"Regular payload: before %d bytes, after %d bytes, net %+d bytes", s.BeforePayloadBytes, s.AfterPayloadBytes, s.NetGrowthBytes)
	line("\nPayload totals count retained regular entries only, excluding hardlink aliases; they are not filesystem or flash usage.\n")

	n := min(10, len(r.PositiveGrowth))
	heading(fmt.Sprintf("Positive growth (showing %d of %d)", n, len(r.PositiveGrowth)))
	if n == 0 {
		line(item + "None.")
	}
	for _, g := range r.PositiveGrowth[:n] {
		line(item+"%s: +%d bytes", literal(g.Path, markdown), g.Bytes)
	}
	line("")

	heading(fmt.Sprintf("Changes (%d)", len(r.Changes)))
	if len(r.Changes) == 0 {
		line(item + "None.")
	}
	for _, c := range r.Changes {
		line(item+"%s %s", c.Kind, literal(c.Path, markdown))
		indent := "    "
		if markdown {
			indent = "  - "
		}
		fields := make([]string, len(c.ChangedFields))
		for i, field := range c.ChangedFields {
			fields[i] = string(field)
		}
		if len(fields) != 0 {
			line(indent+"Changed fields: %s", strings.Join(fields, ", "))
		}
		line(indent+"Before: %s", entry(c.Before, markdown))
		line(indent+"After: %s", entry(c.After, markdown))
	}
	line("")

	heading(fmt.Sprintf("Policy violations (%d)", len(r.Violations)))
	if len(r.Violations) == 0 {
		line(item + "None recorded.")
	}
	for _, v := range r.Violations {
		switch v.Rule {
		case rd.RuleRequiredPath:
			line(item+"required_path: missing explicit candidate path %s", literal(v.Path, markdown))
		case rd.RuleMaxTotalGrowthBytes:
			line(item+"max_total_growth_bytes: net growth %d bytes exceeds limit %d bytes", *v.Actual, *v.Limit)
		}
	}
	line("")

	heading(fmt.Sprintf("Warnings (%d)", len(r.Warnings)))
	if len(r.Warnings) == 0 {
		line(item + "None recorded.")
	}
	for _, w := range r.Warnings {
		line(item+"%s %s: %s key %s", w.Source, literal(w.Path, markdown), w.Code, literal(w.Key, markdown))
	}
	line("")

	heading("Metadata exclusions")
	line(item+"%s", strings.Join(r.MetadataExclusions, ", "))
	line("\nThese metadata are not compared. No changes does not establish firmware equivalence, bootability, compatibility, or security.")
	return b.String()
}

func entry(e *rd.Entry, markdown bool) string {
	if e == nil {
		return "absent"
	}
	s := fmt.Sprintf("kind=%s mode=0%03o uid=%d gid=%d", e.Kind, e.Mode, e.UID, e.GID)
	if e.Kind == rd.KindRegular || e.Kind == rd.KindHardlink {
		s += fmt.Sprintf(" size=%d sha256=%s", e.Size, literal(e.SHA256, markdown))
	}
	if e.Kind == rd.KindSymlink || e.Kind == rd.KindHardlink {
		s += " link_target=" + literal(e.LinkTarget, markdown)
	}
	if e.Kind == rd.KindCharDevice || e.Kind == rd.KindBlockDevice {
		s += fmt.Sprintf(" device_major=%d device_minor=%d", e.DeviceMajor, e.DeviceMinor)
	}
	return s
}
