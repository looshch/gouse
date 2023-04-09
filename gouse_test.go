package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const filesCmpErr = `
========= got:
%s
========= want:
%s`

// End-to-end tests.
func TestMain(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	input, err := os.ReadFile(filepath.Join("testdata", "not_used.input"))
	if err != nil {
		t.Fatal(err)
	}
	tf, err := os.CreateTemp(td, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()
	tf.Write(input)
	gousePath := filepath.Join(td, "gouse")
	if err := exec.Command("go", "build", "-o", gousePath).Run(); err != nil {
		t.Fatal(err)
	}

	tfPath := tf.Name()
	tests := []struct {
		args         []string
		wantFilename string
		wantOutput   string
	}{
		{args: []string{"-v"}, wantOutput: currentVersion + "\n"},
		{args: []string{"-h"}, wantOutput: usageText + "\n"},
		{args: []string{"-w"}, wantOutput: errCannotWriteToStdin.Error() + "\n"},
		{args: []string{tfPath, tfPath}, wantOutput: errMustWriteToFiles.Error() + "\n"},
		{args: []string{}, wantFilename: "not_used.golden"},
		{args: []string{tfPath}, wantFilename: "not_used.golden"},
		// Double processing of the same file so used.golden without not_ prefix.
		{args: []string{"-w", tfPath, tfPath}, wantFilename: "used.golden"},
	}
	for _, tt := range tests {
		args := tt.args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			gouse := exec.Command(gousePath, args...)
			var got []byte
			if len(args) == 0 {
				stdin, err := gouse.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				if _, err = stdin.Write(input); err != nil {
					t.Fatal(err)
				}
				if err = stdin.Close(); err != nil {
					t.Fatal(err)
				}
			}
			mustWrite := len(args) > 1 && args[0] == "-w"
			if mustWrite {
				if err := gouse.Run(); err != nil {
					t.Fatal(err)
				}
				if got, err = os.ReadFile(tfPath); err != nil {
					t.Fatal(err)
				}
			} else {
				wantOutput := tt.wantOutput
				got, _ = gouse.CombinedOutput()
				if len(wantOutput) > 0 {
					if bytes.Equal(got, []byte(wantOutput)) {
						return
					} else {
						t.Errorf(filesCmpErr, got, wantOutput)
					}
				}
			}
			want, err := os.ReadFile(filepath.Join("testdata", tt.wantFilename))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf(filesCmpErr, got, want)
			}
		})
	}
}

func TestToggle(t *testing.T) {
	inputsPaths, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range inputsPaths {
		p := p
		_, filename := filepath.Split(p)
		testName := filename[:len(filename)-len(filepath.Ext(p))]
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			input, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := toggle(input)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", testName+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf(filesCmpErr, got, want)
			}
		})
	}
	t.Cleanup(func() {})
}

func TestGetSymbolsInfoFromBuildErrors(t *testing.T) {
	t.Run("ignore other errors", func(t *testing.T) {
		t.Parallel()
		input := []byte(
			`package p

		         func main() {
		         var (
		         	notUsed0 = false
		         	used0    bool
		         )
		         notUsed1, used1 := "", "", "" // more values than variables
		         _, _ = used0, used1 // no closing brace`,
		)
		want := []SymbolInfo{
			{"notUsed0", 5},
			{"notUsed1", 8},
		}
		got, err := getSymbolsInfoFromBuildErrors(input, notUsedError)
		if err != nil {
			t.Fatal(err)
		}
		for i, info := range got {
			if info.name != want[i].name {
				t.Errorf("got: %s, want: %s", info.name, want[i].name)
			}
			if info.lineNum != want[i].lineNum {
				t.Errorf("got: %d, want: %d", info.lineNum, want[i].lineNum)
			}
		}
	})
}
