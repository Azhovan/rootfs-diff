// Package synthetic supplies the small, offline release-review demonstration.
// It writes tar bytes with Go, never by extracting or executing archive entries.
package synthetic

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Azhovan/rootfs-diff/internal/testfixture"
)

// Generate writes the deterministic demo inputs to dir. Only the named demo
// files are replaced. dir should be an empty temporary or ignored directory.
func Generate(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	before, err := archive(false, false)
	if err != nil {
		return err
	}
	after, err := archive(true, false)
	if err != nil {
		return err
	}
	beforeReordered, err := archive(false, true)
	if err != nil {
		return err
	}
	afterReordered, err := archive(true, true)
	if err != nil {
		return err
	}
	beforeGzip, err := testfixture.Gzip(beforeReordered)
	if err != nil {
		return err
	}
	afterGzip, err := testfixture.Gzip(afterReordered)
	if err != nil {
		return err
	}
	unsafe, err := testfixture.Tar(regular("../escape", "never extracted\n", 0644))
	if err != nil {
		return err
	}
	corruptGzip := slices.Clone(afterGzip)
	corruptGzip[len(corruptGzip)-8] ^= 1 // CRC-32 corruption, after a complete tar.
	for _, file := range []struct {
		name string
		data []byte
	}{
		{"before.tar", before}, {"after.tar", after},
		{"before-reordered.tar.gz", beforeGzip}, {"after-reordered.tar.gz", afterGzip},
		{"unsafe.tar", unsafe}, {"truncated.tar", before[:100]},
		{"corrupt.tar.gz", corruptGzip},
		{"policy.json", []byte("{\"schema_version\":1,\"required_paths\":[\"etc/init.d/S90demo\"],\"max_total_growth_bytes\":0}\n")},
		{"equal-policy.json", []byte("{\"schema_version\":1,\"required_paths\":[\"usr/bin/demo-agent\"],\"max_total_growth_bytes\":4}\n")},
		{"invalid-policy.json", []byte("{\"schema_version\":1,\"schema_version\":1,\"required_paths\":[]}\n")},
	} {
		if err := os.WriteFile(filepath.Join(dir, file.name), file.data, 0600); err != nil {
			return fmt.Errorf("write %s: %w", file.name, err)
		}
	}
	return nil
}

func regular(path, payload string, mode int64) testfixture.Entry {
	return testfixture.Entry{Header: tar.Header{Name: path, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(payload))}, Data: []byte(payload)}
}

func archive(candidate, reordered bool) ([]byte, error) {
	config, payload, target, chainTarget, agentMode := "mode=A\n", "data\n", "/usr/share/old", "usr/share/alias", int64(0755)
	if candidate {
		config, payload, target, chainTarget, agentMode = "mode=B\n", "data grows now\n", "../usr/share/new", "usr/share/payload", 04755
	}
	entries := []testfixture.Entry{
		{Header: tar.Header{Name: ".", Typeflag: tar.TypeDir, Mode: 0755}},
		{Header: tar.Header{Name: "etc/", Typeflag: tar.TypeDir, Mode: 0755}},
		regular("./etc//demo.conf", config, 0644),
		{Header: tar.Header{Name: "etc/current", Typeflag: tar.TypeSymlink, Mode: 0777, Linkname: target}},
		{Header: tar.Header{Name: "usr/", Typeflag: tar.TypeDir, Mode: 0755}},
		regular("usr/bin/demo-agent", "agent\n", agentMode),
		{Header: tar.Header{Name: "usr/share/", Typeflag: tar.TypeDir, Mode: 0755}},
		// Deliberately unlike the terminal regular metadata. These headers cannot
		// describe separate inodes, and the scanner must use the terminal data.
		{Header: tar.Header{Name: "usr/share/chain", Typeflag: tar.TypeLink, Mode: 07777, Uid: 999, Gid: 888, Linkname: chainTarget}},
		{Header: tar.Header{Name: "usr/share/alias", Typeflag: tar.TypeLink, Mode: 0000, Uid: 777, Gid: 666, Linkname: "./usr//share/payload"}},
		regular("usr/share/payload", payload, 0640),
		regular("var/log/boot.log", "ok\n", 0600),
	}
	entries[9].Header.Uid, entries[9].Header.Gid = 12, 34
	if !candidate {
		entries = append(entries, regular("etc/init.d/S90demo", "start\n", 0755))
	}
	for i := range entries {
		entries[i].Header.Format = tar.FormatUSTAR
		entries[i].Header.ModTime = time.Unix(1234567890, 0).UTC()
		entries[i].Header.Uname, entries[i].Header.Gname = "ignored-before", "ignored-before"
		if reordered {
			entries[i].Header.ModTime = time.Unix(1234567900, 0).UTC()
			entries[i].Header.Uname, entries[i].Header.Gname = "ignored-after", "ignored-after"
		}
	}
	if reordered {
		slices.Reverse(entries)
	}
	return testfixture.Tar(entries...)
}
