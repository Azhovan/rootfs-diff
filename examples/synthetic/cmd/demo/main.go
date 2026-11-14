// Command demo generates deterministic inputs and checks a compiled rootfs-diff.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azhovan/rootfs-diff/examples/synthetic"
)

func main() {
	cli := flag.String("cli", "", "path to the compiled rootfs-diff executable")
	output := flag.String("out", "", "optional directory in which to retain demo inputs")
	flag.Parse()
	if *cli == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: demo -cli /path/to/rootfs-diff [-out /temporary/input/directory]")
		os.Exit(2)
	}
	if err := run(*cli, *output); err != nil {
		fmt.Fprintln(os.Stderr, "synthetic demo:", err)
		os.Exit(1)
	}
}

func run(binary, dir string) error {
	var err error
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	if dir == "" {
		dir, err = os.MkdirTemp("", "rootfs-diff-synthetic-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
	}
	if err := synthetic.Generate(dir); err != nil {
		return err
	}
	text, err := synthetic.Check(binary, dir)
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(text); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, "\nSynthetic checks passed: exits 0/1/2, independent findings, encoding/order equivalence, ignores, policy thresholds, repeat stability, and library/CLI agreement.")
	return err
}
