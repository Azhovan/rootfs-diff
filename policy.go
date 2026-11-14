package rootfsdiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const policyMaxBytes int64 = 1 << 20

// ParsePolicy reads one strict JSON policy of at most 1 MiB (1,048,576 bytes),
// including whitespace. It does not close the caller-owned reader. The object
// must contain schema_version (the integer 1) and required_paths (an array of
// exact canonical paths). The optional max_total_growth_bytes is a nonnegative
// int64 integer; omission disables the growth rule, while zero forbids positive
// net growth. Unknown or duplicate object keys, nulls, invalid Unicode, noninteger
// numbers and trailing values are errors. Duplicate required paths are allowed
// and have no extra effect. Paths are validated, never normalized.
//
// Invalid JSON or policy values return an error matching ErrInvalidInput. Input
// over the byte budget returns *LimitError matching ErrLimitExceeded, with Limit
// "policy_bytes". Underlying reader errors remain reachable through errors.Is
// or errors.As. Every failure returns a zero Policy, never a partial policy.
func ParsePolicy(r io.Reader) (Policy, error) {
	if r == nil {
		return Policy{}, invalid("parse policy", "", "reader must not be nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, policyMaxBytes+1))
	if int64(len(data)) > policyMaxBytes {
		return Policy{}, &LimitError{Limit: "policy_bytes", Maximum: policyMaxBytes, Actual: policyMaxBytes + 1, Err: err}
	}
	if err != nil {
		return Policy{}, &Error{Op: "read policy", Err: err}
	}
	// encoding/json replaces invalid UTF-8 and unpaired UTF-16 surrogates.
	// Reject both first so a required filename is never silently rewritten.
	if !utf8.Valid(data) || !policyValidEscapes(data) {
		return Policy{}, invalid("parse policy", "", "JSON must contain valid Unicode without unpaired surrogate escapes")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	fail := func(err error) (Policy, error) {
		return Policy{}, &Error{Op: "parse policy", Kind: ErrorInvalidInput, Err: err}
	}
	token, err := d.Token()
	if err != nil {
		return fail(err)
	}
	if token != json.Delim('{') {
		return Policy{}, invalid("parse policy", "", "expected a JSON object")
	}
	p := Policy{RequiredPaths: []string{}}
	seen := make(map[string]bool, 3)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return fail(err)
		}
		key, ok := token.(string)
		if !ok {
			return Policy{}, invalid("parse policy", "", "expected an object key")
		}
		if seen[key] {
			return Policy{}, invalid("parse policy", "", fmt.Sprintf("duplicate field %q", key))
		}
		seen[key] = true
		switch key {
		case "schema_version", "max_total_growth_bytes":
			token, err := d.Token()
			if err != nil {
				return fail(err)
			}
			number, ok := token.(json.Number)
			if !ok {
				return Policy{}, invalid("parse policy", "", key+" must be an integer")
			}
			n, err := strconv.ParseInt(string(number), 10, 64)
			if err != nil {
				return fail(fmt.Errorf("%s must be an int64 integer: %w", key, err))
			}
			if key == "schema_version" {
				if n != SchemaVersion {
					return Policy{}, invalid("parse policy", "", "unsupported schema version")
				}
				p.SchemaVersion = SchemaVersion
			} else {
				p.MaxTotalGrowthBytes = &n
			}
		case "required_paths":
			token, err := d.Token()
			if err != nil {
				return fail(err)
			}
			if token != json.Delim('[') {
				return Policy{}, invalid("parse policy", "", "required_paths must be an array of strings")
			}
			for d.More() {
				token, err := d.Token()
				if err != nil {
					return fail(err)
				}
				path, ok := token.(string)
				if !ok {
					return Policy{}, invalid("parse policy", "", "required_paths must contain only strings")
				}
				p.RequiredPaths = append(p.RequiredPaths, path)
			}
			if _, err := d.Token(); err != nil { // closing array delimiter
				return fail(err)
			}
		default:
			return Policy{}, invalid("parse policy", "", fmt.Sprintf("unknown field %q", key))
		}
	}
	if _, err := d.Token(); err != nil { // closing object delimiter
		return fail(err)
	}
	if _, err := d.Token(); err != io.EOF {
		if err != nil {
			return fail(err)
		}
		return Policy{}, invalid("parse policy", "", "trailing JSON value")
	}
	if !seen["schema_version"] || !seen["required_paths"] {
		return Policy{}, invalid("parse policy", "", "schema_version and required_paths are required")
	}
	if err := p.Validate(); err != nil {
		return Policy{}, &Error{Op: "parse policy", Err: err}
	}
	return p, nil
}

// policyValidEscapes checks Unicode escape pairs inside JSON strings. The JSON
// decoder remains responsible for syntax; this only closes its lossy-Unicode
// behavior. A backslash escaping another backslash cannot introduce a surrogate.
func policyValidEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) || data[i] != 'u' {
			continue
		}
		n, ok := policyHex4(data[i+1:])
		if !ok {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n < 0xd800 || n > 0xdbff {
			continue
		}
		if len(data)-i <= 6 || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		low, ok := policyHex4(data[i+3:])
		if !ok || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}

func policyHex4(data []byte) (uint16, bool) {
	if len(data) < 4 {
		return 0, false
	}
	var n uint16
	for _, b := range data[:4] {
		n <<= 4
		switch {
		case b >= '0' && b <= '9':
			n += uint16(b - '0')
		case b >= 'a' && b <= 'f':
			n += uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			n += uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return n, true
}

// Evaluate validates all inputs and returns unique violations sorted by rule,
// then path. It does not mutate inputs; returned numeric pointers are independent
// copies. An empty rule set or a passing policy returns an empty slice.
//
// Required paths inspect the full after manifest, including comparison-ignored
// entries. Only explicit entries count: an implicit parent does not satisfy a
// required directory. Presence is sufficient for every kind, including symlinks;
// no links are resolved and no runtime usability or executability is implied.
// A required_path violation identifies the missing path and has nil numeric data.
//
// The growth rule uses diff.Summary.NetGrowthBytes, the comparison's signed net
// uncompressed regular-payload growth after ignore filtering. It does not sum
// PositiveGrowth or recompute totals from the full candidate. Equality with the
// byte limit and shrinking pass, including a zero-byte baseline. A growth failure
// has rule max_total_growth_bytes, empty path, Limit and Actual byte values.
// The caller must supply the diff for its intended comparison and candidate.
//
// Invalid policies, manifests or diffs return a typed ErrInvalidInput error and
// no partial violations. A violation is a successful evaluation, not an error.
func Evaluate(after Manifest, diff Diff, policy Policy) ([]Violation, error) {
	if err := policy.Validate(); err != nil {
		return nil, &Error{Op: "evaluate policy", Err: err}
	}
	if err := after.Validate(); err != nil {
		return nil, &Error{Op: "evaluate candidate", Err: err}
	}
	if err := diff.Validate(); err != nil {
		return nil, &Error{Op: "evaluate diff", Err: err}
	}
	present := make(map[string]bool, len(after.Entries))
	for _, e := range after.Entries {
		present[e.Path] = true
	}
	violations := []Violation{}
	missing := make(map[string]bool)
	for _, path := range policy.RequiredPaths {
		if !present[path] && !missing[path] {
			violations = append(violations, Violation{Rule: RuleRequiredPath, Path: path})
			missing[path] = true
		}
	}
	if policy.MaxTotalGrowthBytes != nil && diff.Summary.NetGrowthBytes > *policy.MaxTotalGrowthBytes {
		limit, actual := *policy.MaxTotalGrowthBytes, diff.Summary.NetGrowthBytes
		violations = append(violations, Violation{Rule: RuleMaxTotalGrowthBytes, Limit: &limit, Actual: &actual})
	}
	slices.SortFunc(violations, func(a, b Violation) int {
		if c := strings.Compare(string(a.Rule), string(b.Rule)); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	return violations, nil
}
