// gouse allows to toggle ‘declared but not used’ errors by using idiomatic
// _ = notUsedVar and leaving a TODO comment.
//
// Usage:
//
//	gouse [-w] [file ...]
//
// By default, gouse accepts code from stdin and writes a toggled version to
// stdout. If any file paths provided with ‘-w’ flag, it writes a toggled
// version back to them, or to stdout if only one path provided without the
// flag.
//
// First it tries to remove fake usages. If there is nothing to remove, it
// tries to build an input and checks a build stdout for the errors. If there
// is any, it creates fake usages for unused variables from the errors.
//
// Examples
//
//	$ gouse
//	...input...
//	notUsed = false
//	...input...
//
//	...output...
//	notUsed = false; _ = notUsed /* TODO: gouse */
//	...output...
//
//	$ gouse main.go
//	...
//	notUsed = false; _ = notUsed /* TODO: gouse */
//	...
//
//	$ gouse -w main.go io.go core.go
//	$ cat main.go io.go core.go
//	...
//	notUsedFromMain = false; _ = notUsedFromMain /* TODO: gouse */
//	...
//	notUsedFromIo = false; _ = notUsedFromIo /* TODO: gouse */
//	...
//	notUsedCore = false; _ = notUsedCore /* TODO: gouse */
//	...
//
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

func usage() {
	fmt.Println("usage: gouse [-w] [file ...]")
	os.Exit(2)
}

var write = flag.Bool("w", false, "write results to files")

func main() {
	flag.Usage = usage
	flag.Parse()

	logFatalWithoutDate := func(msg string) {
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
		log.Fatal(msg)
	}
	paths := flag.Args()
	if len(paths) == 0 {
		if *write {
			logFatalWithoutDate("cannot use -w with standard input")
		}
		if err := handle(os.Stdin, os.Stdout, false); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(paths) > 1 && !*write {
		logFatalWithoutDate("must use -w with multiple paths")
	}
	for _, p := range paths {
		var in *os.File
		var out **os.File
		var flag int
		if *write {
			out = &in
			flag = os.O_RDWR
		} else {
			out = &os.Stdout
			flag = os.O_RDONLY
		}
		in, err := os.OpenFile(p, flag, os.ModeExclusive)
		if err != nil {
			log.Fatal(err)
		}
		defer in.Close()
		if err := handle(in, *out, *write); err != nil {
			log.Fatal(err)
		}
	}
}

// handle manages IO.
func handle(in *os.File, out *os.File, write bool) error {
	code, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("handle: in io.ReadAll: %v", err)
	}
	toggled, err := toggle(code)
	if err != nil {
		return fmt.Errorf("handle: %v", err)
	}
	if write {
		if _, err := out.Seek(0, 0); err != nil {
			return fmt.Errorf("handle: in *File.Seek: %v", err)
		}
		if err := out.Truncate(0); err != nil {
			return fmt.Errorf("handle: in *File.Truncate: %v", err)
		}
	}
	if _, err := out.Write(toggled); err != nil {
		return fmt.Errorf("handle: in *File.Write: %v", err)
	}
	return nil
}

const (
	commentPrefix = "// "

	usagePrefix  = "; _ ="
	usagePostfix = " /* TODO: gouse */"
)

var (
	escapedUsagePostfix = regexp.QuoteMeta(usagePostfix)
	used                = regexp.MustCompile(usagePrefix + ".*" + escapedUsagePostfix)
	usedAndGofmted      = regexp.MustCompile(`\s*_\s*= \w*` + escapedUsagePostfix)
)

// errorInfoRegexp catches position and name of the variable in a build error.
const errorInfoRegexp = `\d+:\d+: \w+`

var errorInfo = regexp.MustCompile(errorInfoRegexp)

var (
	noProviderErr = regexp.MustCompile(errorInfoRegexp + " required module provides package")
	notUsedErr    = regexp.MustCompile(errorInfoRegexp + " declared but not used")
)

// toggle returns toggled code. First it tries to remove fake usages. If there
// is nothing to remove, it creates them.
func toggle(code []byte) ([]byte, error) {
	if used.Match(code) { // used must be before usedAndGofmted because it also removes ‘;’.
		return used.ReplaceAll(code, []byte("")), nil
	}
	if usedAndGofmted.Match(code) {
		return usedAndGofmted.ReplaceAll(code, []byte("")), nil
	}

	lines := bytes.Split(code, []byte("\n"))
	// Check for problematic imports and comment them out if any, storing commented
	// out lines numbers to commentedLinesNums.
	noProviderVarsInfo, err := errorVarsInfo(code, noProviderErr)
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
	notUsedVarsInfo, err := errorVarsInfo(bytes.Join(lines, []byte("\n")), notUsedErr)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	for _, info := range notUsedVarsInfo {
		l := &lines[info.lineNum]
		*l = append(*l, []byte(usagePrefix+info.name+usagePostfix)...)
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
