# Policy version 1

`policy-v1.json` is a valid policy for `rootfsdiff.ParsePolicy`. The executable
`ExampleParsePolicy` and `ExampleEvaluate` show the public API.

The JSON object must contain integer `schema_version: 1` and `required_paths`,
an array of exact canonical paths relative to the archived filesystem root.
Unknown or duplicate fields, incorrect types (including null), invalid Unicode,
noncanonical paths, trailing values, negative byte limits and integers outside
`int64` return `ErrInvalidInput`. Repeated required paths are allowed and have no
additional effect. The parser accepts
at most 1 MiB (1,048,576 bytes), including whitespace; excess returns a
`*rootfsdiff.LimitError` with limit `policy_bytes`, matching `ErrLimitExceeded`.
Use `errors.Is` or `errors.As` to inspect the underlying cause of a reader
failure. The caller owns the reader and is responsible for closing it.

`max_total_growth_bytes` is optional. Omission disables the growth check; zero
forbids positive growth. Its unit is bytes of uncompressed regular-file payload,
not filesystem allocation or flash size. The check uses the comparison's signed
net growth after ignores, including removals. Equality and shrinking pass. It
does not sum positive per-file growth or divide by the baseline size.

Required-path checks inspect the full candidate, including entries hidden by
ignore rules. Only explicitly recorded entries count. For example, a file named
`etc/device.conf` does not satisfy a required path of `etc` unless the archive
also records that directory. Any entry kind satisfies presence. Symlinks are
not followed, and presence does not establish executability, target existence
or usability at runtime.

`Evaluate` also validates policies, manifests and diffs built by callers. It
returns unique structured violations sorted by rule, then path, without changing
the inputs. A missing path uses rule `required_path`, the exact missing path and
nil numeric values. A growth violation uses rule `max_total_growth_bytes`, an
empty path and `Limit`/`Actual` byte values. Finding a violation is a successful
evaluation and does not return an error. The caller must supply the diff for
the intended comparison and the full candidate manifest.
