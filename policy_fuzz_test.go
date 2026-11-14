package rootfsdiff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	rd "github.com/Azhovan/rootfs-diff"
)

func FuzzParsePolicy(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"schema_version":1,"required_paths":[]}`),
		[]byte(`{"schema_version":1,"required_paths":["etc/agent"],"max_total_growth_bytes":0}`),
		[]byte(`{"schema_version":1,"required_paths":["a","a"],"max_total_growth_bytes":9223372036854775807}`),
		[]byte(`{"schema_version":1,"required_paths":["\ud83d\ude00"]}`),
		[]byte(`{"schema_version":1,"required_paths":["\ud800"]}`),
		[]byte(`{"schema_version":1,"required_paths":[],"schema_version":1}`),
		[]byte(`{"schema_version":1,"required_paths":[]} {}`),
		[]byte("{\"schema_version\":1,\"required_paths\":[\"\xff\"]}"),
		nil,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		// Fuzz syntax and validation without allowing corpus growth to turn a
		// parser check into unbounded allocation or deep-input work.
		if len(input) > 4096 {
			return
		}
		p, err := rd.ParsePolicy(bytes.NewReader(input))
		if err != nil {
			if !errors.Is(err, rd.ErrInvalidInput) {
				t.Fatalf("bounded in-memory policy produced unclassified error: %v", err)
			}
			return
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("successful parse returned an invalid policy: %v", err)
		}
		encoded, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip, err := rd.ParsePolicy(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("valid parsed policy cannot round-trip: %v", err)
		}
		again, err := json.Marshal(roundTrip)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, again) {
			t.Fatalf("policy changed on round-trip:\n%s\n%s", encoded, again)
		}
	})
}
