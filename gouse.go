// gouse allows to toggle ‘declared but not used’ errors by using idiomatic
// _ = notUsedVar and leaving a TODO comment.
//
// Usage:
//
//	gouse [-w] [file ...]
//
// By default, gouse accepts code from stdin and writes a toggled version to
// stdout. If any file paths provided, it takes code from them and writes a
// toggled version to stdout unless ‘-w’ flag is passed — then it will write
// back to the file paths.
//
// First it tries to remove fake usages. If there is nothing to remove, it tries
// to build an input and checks a build stdout for the errors. If there is any,
// it creates fake usages for unused variables from the errors.
//
// Examples
//
//	$ gouse
//	...input...
//
//	notUsed = false
//
//	...input...
//	...output...
//
//	notUsed = false; _ = notUsed /* TODO: gouse */
//
//	...output...
//
//	$ gouse main.go
//	...output...
//
//	notUsed = false; _ = notUsed /* TODO: gouse */
//
//	...output...
//
//	$ gouse -w main.go io.go core.go
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

	paths := flag.Args()
	if len(paths) == 0 {
		if *write {
			log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
			log.Fatal("cannot use -w with standard input")
		}
		if err := handle(os.Stdin, os.Stdout, false); err != nil {
			log.Fatal(err)
		}
		return
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
		return err
	}
	toggled, err := toggle(code)
	if err != nil {
		return err
	}
	if write {
		if _, err := out.Seek(0, 0); err != nil {
			return err
		}
		if err := out.Truncate(0); err != nil {
			return err
		}
	}
	if _, err := out.Write(toggled); err != nil {
		return err
	}
	return nil
}

const (
	COMMENT_PREFIX = "// "

	USAGE_PREFIX  = "; _ ="
	USAGE_POSTFIX = " /* TODO: gouse */"
)

var commentPrefixPadding = len([]rune(COMMENT_PREFIX))

var (
	escapedUsagePostfix = regexp.QuoteMeta(USAGE_POSTFIX)

	used           = regexp.MustCompile(USAGE_PREFIX + ".*" + escapedUsagePostfix)
	usedAndGofmted = regexp.MustCompile(`\s*_\s*= \w*` + escapedUsagePostfix)
)

// ERROR_INFO_REGEXP catches position and name of the variable in a build error.
const ERROR_INFO_REGEXP = `\d+:\d+: \w+`

var (
	errorInfo = regexp.MustCompile(ERROR_INFO_REGEXP)

	noProviderErr = regexp.MustCompile(ERROR_INFO_REGEXP + " required module provides package")
	notUsedErr    = regexp.MustCompile(ERROR_INFO_REGEXP + " declared but not used")
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
	noProviderVarsInfo, err := getVarsInfoFrom(code, noProviderErr)
	if err != nil {
		return nil, err
	}
	var commentedLinesNums []int
	for _, info := range noProviderVarsInfo {
		l := &lines[info.lineNum]
		*l = append([]byte(COMMENT_PREFIX), *l...)
		commentedLinesNums = append(commentedLinesNums, info.lineNum)
	}
	// Check for ‘declared but not used’ errors and create fake usages for them if
	// any.
	notUsedVarsInfo, err := getVarsInfoFrom(bytes.Join(lines, []byte("\n")), notUsedErr)
	if err != nil {
		return nil, err
	}
	for _, info := range notUsedVarsInfo {
		l := &lines[info.lineNum]
		*l = append(*l, []byte(USAGE_PREFIX+info.name+USAGE_POSTFIX)...)
	}
	// Un-comment commented out lines.
	for _, line := range commentedLinesNums {
		l := &lines[line]
		uncommentedLine := []rune(string(*l))[commentPrefixPadding:]
		*l = []byte(string(uncommentedLine))
	}
	return bytes.Join(lines, []byte("\n")), nil
}

type VarInfo struct {
	name    string
	lineNum int
}

// getVarsInfoFrom tries to build code and checks a build stdout for errors
// catched by r. If any, it returns a slice of tuples with a line and a name of
// every catched symbol.
func getVarsInfoFrom(code []byte, r *regexp.Regexp) ([]VarInfo, error) {
	td, err := os.MkdirTemp(os.TempDir(), "gouse")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(td)
	tf, err := os.CreateTemp(td, "*.go")
	if err != nil {
		return nil, err
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
			return nil, err
		}
		info = append(info, VarInfo{
			name:    varInfo[2],
			lineNum: lineNum - 1, // An adjustment for 0-based count.
		})
	}
	return info, nil
}
