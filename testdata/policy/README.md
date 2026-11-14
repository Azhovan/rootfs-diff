# Policy version 1

`policy-v1.json` is a valid policy for `rootfsdiff.ParsePolicy`. The executable
`ExampleParsePolicy` and `ExampleEvaluate` show the public API.

The JSON object requires integer `schema_version: 1` and `required_paths`, an
array of exact canonical root-relative paths. Unknown or duplicate fields, bad
types (including null), invalid Unicode, noncanonical paths, trailing values,
negative byte limits and integers outside int64 fail as `ErrInvalidInput`.
Repeated required paths are allowed and have no extra effect. The parser accepts
at most 1 MiB (1,048,576 bytes), including whitespace; excess returns a
`*rootfsdiff.LimitError` with limit `policy_bytes`, matching `ErrLimitExceeded`.
Reader failures retain their underlying identity through `errors.Is` or
`errors.As`. The caller retains ownership of the reader.

`max_total_growth_bytes` is optional. Omission disables the growth check; zero
forbids positive growth. Its unit is bytes of uncompressed regular-file payload,
not filesystem allocation or flash size. The check uses the comparison's signed
net growth after ignores, including removals. Equality and shrinking pass. It
does not sum positive per-file growth or divide by the baseline size.

Required paths inspect the full candidate, including entries hidden by
comparison ignores. Only explicit entries count: an implicit directory does not
satisfy its own required path. Any entry kind satisfies presence. Symlinks are
not followed; presence says nothing about executability, target existence or
runtime usability.

`Evaluate` validates caller-built policies, manifests and diffs too. It returns
unique structured violations sorted by rule then path without changing inputs.
A missing path uses rule `required_path`, the exact missing path and nil numeric
values. A growth failure uses rule `max_total_growth_bytes`, empty path and
`Limit`/`Actual` byte values. Violations are a successful result, not an error.
The caller supplies the diff for the intended comparison and full candidate.
