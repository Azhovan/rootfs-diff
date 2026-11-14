package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Azhovan/rootfs-diff/examples/synthetic"
)

func TestSyntheticReleaseReview(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository")
	}
	root := filepath.Dir(filepath.Dir(file))
	work := t.TempDir()
	binary := filepath.Join(work, "rootfs-diff")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "./cmd/rootfs-diff")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build actual executable: %v\n%s", err, out)
	}
	inputs := filepath.Join(work, "inputs")
	if err := synthetic.Generate(inputs); err != nil {
		t.Fatal(err)
	}
	text, err := synthetic.Check(binary, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range []string{"etc/init.d/S90demo", "usr/share/alias", "usr/share/chain", "04755"} {
		if !bytes.Contains(text, []byte(finding)) {
			t.Fatalf("readable demo omits %q:\n%s", finding, text)
		}
	}
	if _, err := os.Stat(filepath.Join(work, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unsafe entry appeared outside its archive: %v", err)
	}
}

func TestSyntheticFixturesAreDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, dir := range []string{a, b} {
		if err := synthetic.Generate(dir); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 10 {
		t.Fatalf("got %d files, want ten documented demo inputs", len(files))
	}
	for _, file := range files {
		first, err := os.ReadFile(filepath.Join(a, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(b, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Errorf("fixture %s differs between runs", file.Name())
		}
	}
}
