package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

// osOpenFile is a type of os.OpenFile.
type osOpenFile func(name string, flag int, perm os.FileMode) (file, error)

// openFile is a wrapper around os.OpenFile.
var openFile osOpenFile = func(name string, flag int, perm os.FileMode) (file, error) {
	file, err := os.OpenFile(name, flag, perm)
	return file, err
}

// config represents parsed CLI arguments.
type config struct {
	version bool
	write   bool
	paths   []string
}

// parseArgs accepts args, parses them and returns config, parsing message and
// err. flag.ErrHelp is a special error which is returned on -h, -help, --help
// and when misused.
func parseArgs(args []string) (*config, string, error) {
	c := new(config)
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	var out bytes.Buffer
	flags.SetOutput(&out)
	flags.BoolVar(&c.version, "v", false, "show version")
	flags.BoolVar(&c.write, "w", false, "write results to files")
	flags.Usage = func() { out.Write([]byte(usageText)) }
	if err := flags.Parse(args); err != nil {
		return nil, out.String(), err
	}
	// flags.Args must be called after flags.Parse.
	c.paths = flags.Args()
	return c, out.String(), nil
}

// toggleFile takes code from in, toggles it, deletes contents of out if it’s in,
// and writes the toggled version to out.
func toggleFile(ctx context.Context, in, out file) error {
	code, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("toggleFile: in io.ReadAll: %v", err)
	}
	toggled, err := toggle(ctx, code)
	if err != nil {
		return fmt.Errorf("toggleFile: %v", err)
	}
	if out == in {
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("toggleFile: in *File.Seek: %v", err)
		}
		if err := out.Truncate(0); err != nil {
			return fmt.Errorf("toggleFile: in *File.Truncate: %v", err)
		}
	}
	if _, err := out.Write(toggled); err != nil {
		return fmt.Errorf("toggleFile: in *File.Write: %v", err)
	}
	return nil
}
