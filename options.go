package rootfsdiff

const (
	DefaultMaxEntries      int64 = 100_000
	DefaultMaxArchiveBytes int64 = 8 << 30
	DefaultMaxFileBytes    int64 = 2 << 30
	DefaultMaxPathBytes    int64 = 4_096
)

// ScanOptions limits decoded input, including headers, extensions and padding.
// Zero fields select defaults. Positive values override them; negatives fail.
// MaxPathBytes applies independently to final entry paths and link targets.
type ScanOptions struct {
	MaxEntries      int64 `json:"max_entries"`
	MaxArchiveBytes int64 `json:"max_archive_bytes"`
	MaxFileBytes    int64 `json:"max_file_bytes"`
	MaxPathBytes    int64 `json:"max_path_bytes"`
}

// WithDefaults validates options and returns an independent effective value.
func (o ScanOptions) WithDefaults() (ScanOptions, error) {
	fields := []struct {
		name string
		ptr  *int64
		def  int64
	}{
		{"max_entries", &o.MaxEntries, DefaultMaxEntries},
		{"max_archive_bytes", &o.MaxArchiveBytes, DefaultMaxArchiveBytes},
		{"max_file_bytes", &o.MaxFileBytes, DefaultMaxFileBytes},
		{"max_path_bytes", &o.MaxPathBytes, DefaultMaxPathBytes},
	}
	for _, f := range fields {
		if *f.ptr < 0 {
			return ScanOptions{}, invalid("scan options", "", f.name+" must be positive or zero for default")
		}
		if *f.ptr == 0 {
			*f.ptr = f.def
		}
	}
	return o, nil
}

func (o ScanOptions) Validate() error {
	_, err := o.WithDefaults()
	return err
}

// CompareOptions applies literal canonical paths to both sides. Prefixes match
// themselves and descendants separated by '/', never neighboring names.
// Input order and duplicate rules are allowed and have no semantic effect.
type CompareOptions struct {
	IgnorePaths    []string `json:"ignore_paths"`
	IgnorePrefixes []string `json:"ignore_prefixes"`
}

func (o CompareOptions) Validate() error {
	for _, paths := range [][]string{o.IgnorePaths, o.IgnorePrefixes} {
		for _, p := range paths {
			if err := ValidatePath(p); err != nil {
				return err
			}
		}
	}
	return nil
}
