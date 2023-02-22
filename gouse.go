// gouse toggles ‘declared but not used’ errors by using idiomatic
// _ = notUsedVar and leaving a TODO comment.
//
// Usage:
//
//	gouse [-w] [file paths...]
//
// By default, gouse accepts code from stdin or from a file provided as a path
// argument and writes the toggled version to stdout. ‘-w’ flag writes the
// result back to the file. If multiple paths provided, gouse writes results
// back to them regardless of ‘-w’ flag.
//
// First it tries to remove previously created fake usages. If there is nothing
// to remove, it tries to build an input and checks the build stdout for
// ‘declared but not used’ errors. If there is any, it creates fake usages for
// unused variables from the errors.
//
// Examples
//
//	$ gouse
//	...input...
//	notUsed = true
//	...input...
//
//	...output...
//	notUsed = true; _ = notUsed /* TODO: gouse */
//	...output...
//
//	$ gouse main.go
//	...
//	notUsed = true; _ = notUsed /* TODO: gouse */
//	...
//
//	$ gouse main.go io.go core.go
//	$ cat main.go io.go core.go
//	...
//	notUsedFromMain = true; _ = notUsedFromMain /* TODO: gouse */
//	...
//	notUsedFromIo = true; _ = notUsedFromIo /* TODO: gouse */
//	...
//	notUsedFromCore = true; _ = notUsedFromCore /* TODO: gouse */
//	...
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const usageText = "usage: gouse [-w] [file paths...]"

func usage() {
	fmt.Println(usageText)
	os.Exit(2)
}

var write = flag.Bool("w", false, "write results to files")

func main() {
	flag.Usage = usage
	flag.Parse()

	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	paths := flag.Args()
	writeToFile := *write
	if len(paths) == 0 {
		if writeToFile {
			log.Fatal("cannot use -w with standard input")
		}
		if err := toggleInput(os.Stdin, os.Stdout, false); err != nil {
			log.Fatal(err)
		}
		return
	}
	writeToFiles := writeToFile || len(paths) > 1
	for _, p := range paths {
		var in *os.File
		var out **os.File
		var access int
		if writeToFiles {
			out = &in
			access = os.O_RDWR
		} else {
			out = &os.Stdout
			access = os.O_RDONLY
		}
		in, err := os.OpenFile(p, access, os.ModeExclusive)
		if err != nil {
			log.Fatal(err)
		}
		defer in.Close()
		if err := toggleInput(in, *out, writeToFiles); err != nil {
			log.Fatal(err)
		}
	}
}

// toggleInput takes code from in, toggles it, deletes contents of out if it’s a
// file, and writes the toggled version to out.
func toggleInput(in *os.File, out *os.File, writeToFile bool) error {
	code, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("toggleInput: in io.ReadAll: %v", err)
	}
	toggled, err := toggle(code)
	if err != nil {
		return fmt.Errorf("toggleInput: %v", err)
	}
	if writeToFile {
		if _, err := out.Seek(0, 0); err != nil {
			return fmt.Errorf("toggleInput: in *File.Seek: %v", err)
		}
		if err := out.Truncate(0); err != nil {
			return fmt.Errorf("toggleInput: in *File.Truncate: %v", err)
		}
	}
	if _, err := out.Write(toggled); err != nil {
		return fmt.Errorf("toggleInput: in *File.Write: %v", err)
	}
	return nil
}

const (
	commentPrefix = "// "

	fakeUsagePrefix = "; _ ="
	fakeUsageSuffix = " /* TODO: gouse */"
)

var (
	escapedFakeUsageSuffix = regexp.QuoteMeta(fakeUsageSuffix)
	fakeUsage              = regexp.MustCompile(fakeUsagePrefix + ".*" + escapedFakeUsageSuffix)
	fakeUsageAfterGofmt    = regexp.MustCompile(`\s*_\s*= \w*` + escapedFakeUsageSuffix)
)

// errorInfoRegexp catches position and name of the variable in a build error.
const errorInfoRegexp = `\d+:\d+: \w+`

var errorInfo = regexp.MustCompile(errorInfoRegexp)

var (
	noProviderError = regexp.MustCompile(errorInfoRegexp + " required module provides package")
	notUsedError    = regexp.MustCompile(errorInfoRegexp + " declared but not used")
)

// toggle returns toggled code. First it tries to remove previosly created fake
// usages. If there is nothing to remove, it creates them.
func toggle(code []byte) ([]byte, error) {
	if fakeUsage.Match(code) { // fakeUsage must be before fakeUsageAfterGofmt because it also removes the leading ‘;’.
		return fakeUsage.ReplaceAll(code, []byte("")), nil
	}
	if fakeUsageAfterGofmt.Match(code) {
		return fakeUsageAfterGofmt.ReplaceAll(code, []byte("")), nil
	}

	lines := bytes.Split(code, []byte("\n"))
	// Check for problematic imports and comment them out if any, storing commented
	// out lines numbers to commentedLinesNums.
	noProviderVarsInfo, err := errorVarsInfo(code, noProviderError)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	var commentedLinesNums []int
	for _, info := range noProviderVarsInfo {
		l := &lines[info.lineNum]
		*l = append([]byte(commentPrefix), *l...)
		commentedLinesNums = append(commentedLinesNums, info.lineNum)
	}
	// Check for ‘declared but not used’ errors and create fake usages for them if
	// any.
	notUsedVarsInfo, err := errorVarsInfo(bytes.Join(lines, []byte("\n")), notUsedError)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	for _, info := range notUsedVarsInfo {
		l := &lines[info.lineNum]
		*l = append(*l, []byte(fakeUsagePrefix+info.name+fakeUsageSuffix)...)
	}
	// Un-comment commented out lines.
	for _, line := range commentedLinesNums {
		l := &lines[line]
		uncommentedLine := []rune(string(*l))[len([]rune(commentPrefix)):]
		*l = []byte(string(uncommentedLine))
	}
	return bytes.Join(lines, []byte("\n")), nil
}

type VarInfo struct {
	name    string
	lineNum int
}

// errorVarsInfo tries to build code and checks a build stdout for errors
// catched by r. If any, it returns a slice of tuples with a line and a name of
// every catched symbol.
func errorVarsInfo(code []byte, r *regexp.Regexp) ([]VarInfo, error) {
	td, err := os.MkdirTemp(os.TempDir(), "gouse")
	if err != nil {
		return nil, fmt.Errorf("errorVarsInfo: in os.MkdirTemp: %v", err)
	}
	defer os.RemoveAll(td)
	tf, err := os.CreateTemp(td, "*.go")
	if err != nil {
		return nil, fmt.Errorf("errorVarsInfo: in os.CreateTemp: %v", err)
	}
	defer tf.Close()
	tf.Write(code)
	bo, err := exec.Command("go", "build", "-o", os.DevNull, tf.Name()).CombinedOutput()
	if err == nil {
		return nil, nil
	}
	buildErrors := strings.Split(string(bo), "\n")
	var info []VarInfo
	for _, e := range buildErrors {
		if !r.MatchString(e) {
			continue
		}
		varInfo := strings.Split(errorInfo.FindString(e), ":")
		lineNum, err := strconv.Atoi(varInfo[0])
		if err != nil {
			return nil, fmt.Errorf("errorVarsInfo: in strconv.Atoi: %v", err)
		}
		info = append(info, VarInfo{
			name:    varInfo[2],
			lineNum: lineNum - 1, // An adjustment for 0-based count.
		})
	}
	return info, nil
}
