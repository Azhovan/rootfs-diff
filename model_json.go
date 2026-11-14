package rootfsdiff

import "encoding/json"

// These marshalers fix empty arrays at the model boundary, including nested
// changes. They do not validate or mutate caller values; call Validate first.
func array[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (m Manifest) MarshalJSON() ([]byte, error) {
	type plain Manifest
	m.Entries, m.Warnings = array(m.Entries), array(m.Warnings)
	return json.Marshal(plain(m))
}

func (c Change) MarshalJSON() ([]byte, error) {
	type plain Change
	c.ChangedFields = array(c.ChangedFields)
	return json.Marshal(plain(c))
}

func (d Diff) MarshalJSON() ([]byte, error) {
	type plain Diff
	d.Changes, d.PositiveGrowth, d.Warnings = array(d.Changes), array(d.PositiveGrowth), array(d.Warnings)
	return json.Marshal(plain(d))
}

func (p Policy) MarshalJSON() ([]byte, error) {
	type plain Policy
	p.RequiredPaths = array(p.RequiredPaths)
	return json.Marshal(plain(p))
}

func (r Report) MarshalJSON() ([]byte, error) {
	type plain Report
	r.Changes, r.PositiveGrowth = array(r.Changes), array(r.PositiveGrowth)
	r.Violations, r.Warnings = array(r.Violations), array(r.Warnings)
	r.MetadataExclusions = array(r.MetadataExclusions)
	return json.Marshal(plain(r))
}

func (o CompareOptions) MarshalJSON() ([]byte, error) {
	type plain CompareOptions
	o.IgnorePaths, o.IgnorePrefixes = array(o.IgnorePaths), array(o.IgnorePrefixes)
	return json.Marshal(plain(o))
}
