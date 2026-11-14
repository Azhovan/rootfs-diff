package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	rootfsdiff "github.com/Azhovan/rootfs-diff"
)

func TestExampleReport(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	var got rootfsdiff.Report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Independent expectations: build ID payload is ignored, the five-byte
	// config grows to six bytes, and the symlink contributes no payload bytes.
	wantSummary := rootfsdiff.Summary{
		BeforeEntries: 2, AfterEntries: 2, Modified: 2,
		BeforePayloadBytes: 5, AfterPayloadBytes: 6, NetGrowthBytes: 1,
	}
	if got.SchemaVersion != 1 || got.Summary != wantSummary {
		t.Fatalf("schema/summary: %d %+v", got.SchemaVersion, got.Summary)
	}
	if len(got.Violations) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("unexpected violations/warnings: %+v %+v", got.Violations, got.Warnings)
	}
	if len(got.Changes) != 2 {
		t.Fatalf("changes: %+v", got.Changes)
	}
	config, link := got.Changes[0], got.Changes[1]
	if config.Path != "etc/device.conf" || config.Kind != rootfsdiff.ChangeModified ||
		!reflect.DeepEqual(config.ChangedFields, []rootfsdiff.Field{rootfsdiff.FieldMode, rootfsdiff.FieldSize, rootfsdiff.FieldSHA256}) ||
		config.Before == nil || config.After == nil {
		t.Fatalf("config change: %+v", config)
	}
	if config.Before.Mode != 0640 || config.After.Mode != 06750 ||
		config.Before.Size != 5 || config.After.Size != 6 ||
		config.After.UID != 100 || config.After.GID != 200 ||
		config.Before.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" ||
		config.After.SHA256 != "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03" {
		t.Fatalf("config metadata: before=%+v after=%+v", config.Before, config.After)
	}
	if link.Path != "usr/bin/agent" || link.Kind != rootfsdiff.ChangeModified ||
		!reflect.DeepEqual(link.ChangedFields, []rootfsdiff.Field{rootfsdiff.FieldLinkTarget}) ||
		link.Before == nil || link.After == nil ||
		link.Before.LinkTarget != "../lib/agent-v1" || link.After.LinkTarget != "../lib/agent-v2" {
		t.Fatalf("symlink change: %+v", link)
	}
	if !reflect.DeepEqual(got.PositiveGrowth, []rootfsdiff.Growth{{Path: "etc/device.conf", Bytes: 1}}) {
		t.Fatalf("growth: %+v", got.PositiveGrowth)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExamplePreservesCancellationAndWriterErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	want := errors.New("destination unavailable")
	if err := run(context.Background(), failingWriter{want}); !errors.Is(err, want) {
		t.Fatalf("writer error: %v", err)
	}
}

func TestTypedErrorDiagnostics(t *testing.T) {
	// Exercise actual public API errors rather than constructing the types.
	_, invalid := rootfsdiff.ParsePolicy(strings.NewReader(`{"schema_version":1,"required_paths":[],"typo":0}`))
	_, limited := rootfsdiff.ParsePolicy(strings.NewReader(strings.Repeat(" ", (1<<20)+1)))
	for _, tc := range []struct {
		err    error
		prefix string
	}{
		{invalid, "invalid input during parse policy"},
		{limited, "policy_bytes exceeded: observed 1048577, maximum 1048576"},
		{fmt.Errorf("scan: %w", context.Canceled), "canceled:"},
	} {
		if tc.err == nil {
			t.Fatal("expected public API error")
		}
		if !strings.HasPrefix(describeError(tc.err), tc.prefix) {
			t.Errorf("diagnostic for %v: %q", tc.err, describeError(tc.err))
		}
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
