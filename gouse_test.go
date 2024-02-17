package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const filesCmpErr = `
========= got:
%s
========= want:
%s`

type fakeFile struct {
	file

	contents *bytes.Buffer
}

func newFakeFile(buf ...byte) *fakeFile {
	newBuf := bytes.NewBuffer(buf)
	if buf == nil {
		newBuf = new(bytes.Buffer)
	}
	f := fakeFile{contents: newBuf}
	return &f
}

func (f *fakeFile) Read(b []byte) (int, error) {
	n, err := f.contents.Read(b)
	return n, err
}

func (f *fakeFile) Write(b []byte) (int, error) {
	n, err := f.contents.Write(b)
	return n, err
}

func (f *fakeFile) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (f *fakeFile) Truncate(size int64) error {
	f.contents = bytes.NewBuffer([]byte{})
	return nil
}

func (f *fakeFile) Close() error {
	return nil
}

func TestRun(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "not_used.input"))
	var openInput osOpenFile = func(name string, flag int, perm os.FileMode) (file, error) {
		return newFakeFile(input...), nil
	}
	if err != nil {
		t.Fatal(err)
	}
	mockPath := "filename"
	tests := []struct {
		args         []string
		wantFilename string
		wantOutput   string
		wantStatus   int
	}{
		{
			args:       []string{"-v"},
			wantOutput: currentVersion + "\n",
			wantStatus: 0,
		},
		{
			args:       []string{"-h"},
			wantOutput: usageText + "\n",
			wantStatus: 2,
		},
		{
			args:       []string{"-w"},
			wantOutput: errorLogPrefix + errCannotWriteToStdin.Error() + "\n",
			wantStatus: 1,
		},
		{
			args:       []string{mockPath, mockPath},
			wantOutput: errorLogPrefix + errMustWriteToFiles.Error() + "\n",
			wantStatus: 1,
		},
		{
			args:         []string{},
			wantFilename: "not_used.golden",
			wantStatus:   0,
		},
		{
			args:         []string{mockPath},
			wantFilename: "not_used.golden",
			wantStatus:   0,
		},
		{
			// Double processing of the same file must return to exact previous state.
			args:         []string{"-w", mockPath, mockPath},
			wantFilename: "not_used.input",
			wantStatus:   0,
		},
	}
	for _, tt := range tests {
		test := tt
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			args := test.args
			var (
				stdin  = newFakeFile()
				stdout = newFakeFile()
				stderr = newFakeFile()
			)
			if len(args) == 0 {
				if _, err = stdin.Write(input); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			status := run(ctx, args, stdin, stdout, stderr, openInput)
			got, err := io.ReadAll(stdout)
			if err != nil {
				t.Fatal(err)
			}
			mustWrite := len(args) > 1 && args[0] == "-w"
			if mustWrite {
				got = input
			} else {
				wantOutput := test.wantOutput
				gotFromStderr, err := io.ReadAll(stderr)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, gotFromStderr...)
				if len(wantOutput) > 0 {
					if bytes.Equal(got, []byte(wantOutput)) {
						return
					} else {
						t.Errorf(filesCmpErr, got, wantOutput)
					}
				}
			}
			want, err := os.ReadFile(filepath.Join("testdata", test.wantFilename))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf(filesCmpErr, got, want)
			}
			if test.wantStatus != status {
				t.Errorf("got: %d, want: %d", status, test.wantStatus)
			}
		})
	}
}
