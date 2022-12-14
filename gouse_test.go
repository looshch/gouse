package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const FILES_CMP_ERR = `
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
		{args: []string{}, wantFilename: "not_used.golden"},
		{args: []string{"-w"}, wantOutput: "cannot use -w with standard input\n"},
		{args: []string{tfPath}, wantFilename: "not_used.golden"},
		{args: []string{"-w", tfPath, tfPath}, wantFilename: "used.golden"}, // Double processing of the same file so used.golden without not_ prefix.
		{args: []string{tfPath, tfPath}, wantOutput: "must use -w with multiple paths\n"},
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
			isWriteFlagValid := len(args) > 1
			isWriteFlagUsed := isWriteFlagValid && args[0] == "-w"
			if isWriteFlagUsed {
				if err := gouse.Run(); err != nil {
					t.Fatal(err)
				}
				if got, err = os.ReadFile(tfPath); err != nil {
					t.Fatal(err)
				}
			} else {
				wantOutput := tt.wantOutput
				if got, err = gouse.CombinedOutput(); err != nil {
					if bytes.Equal(got, []byte(wantOutput)) {
						return
					} else {
						t.Errorf(FILES_CMP_ERR, got, wantOutput)
					}
				}
			}
			want, e := os.ReadFile(filepath.Join("testdata", tt.wantFilename))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, want) {
				t.Errorf(FILES_CMP_ERR, got, want)
			}
		})
	}
}

func TestToggle(t *testing.T) {
	inputsPaths, e := filepath.Glob(filepath.Join("testdata", "*.input"))
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range inputsPaths {
		p := p
		_, filename := filepath.Split(p)
		testName := filename[:len(filename)-len(filepath.Ext(p))]
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			input, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			got, e := toggle(input)
			if e != nil {
				t.Fatal(e)
			}
			want, e := os.ReadFile(filepath.Join("testdata", testName+".golden"))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, want) {
				t.Errorf(FILES_CMP_ERR, got, want)
			}
		})
	}
	t.Cleanup(func() {})
}

func TestGetVarsInfoFrom(t *testing.T) {
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
		want := []VarInfo{
			{"notUsed0", 5},
			{"notUsed1", 8},
		}
		got, e := getVarsInfoFrom(input, notUsedErr)
		if e != nil {
			t.Fatal(e)
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
