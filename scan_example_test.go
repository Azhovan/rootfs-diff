package rootfsdiff_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"

	rootfsdiff "github.com/Azhovan/rootfs-diff"
)

func ExampleScan() {
	// Archive creation is only for this self-contained example. In an
	// application, pass an opened local tar or tar.gz file as the reader.
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	if err := w.WriteHeader(&tar.Header{Name: "etc/device.conf", Typeflag: tar.TypeReg, Mode: 0640, Uid: 100, Gid: 200, Size: 5}); err != nil {
		panic(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	manifest, err := rootfsdiff.Scan(context.Background(), &archive, rootfsdiff.ScanOptions{})
	if err != nil {
		panic(err)
	}
	for _, entry := range manifest.Entries {
		fmt.Printf("%s %s %04o %d:%d %d %s\n", entry.Path, entry.Kind, entry.Mode, entry.UID, entry.GID, entry.Size, entry.SHA256)
	}
	// Output:
	// etc/device.conf regular 0640 100:200 5 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
}
