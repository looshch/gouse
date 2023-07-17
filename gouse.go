// gouse toggles ‘declared and not used’ errors by using idiomatic
// _ = notUsedVar and leaving a TODO comment.
//
// Usage:
//
//	gouse [-w] [file paths...]
//
// By default, gouse accepts code from stdin or from a file provided as a path
// argument and writes the toggled version to stdout. ‘-w’ flag writes the
// result back to the file. If multiple paths provided, ‘-w’ flag is required.
//
// First it tries to remove previously created fake usages. If there is nothing
// to remove, it tries to build an input and checks the build stdout for
// ‘declared and not used’ errors. If there is any, it creates fake usages for
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
//	$ gouse -w main.go io.go core.go
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
	"errors"
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

var errCannotWriteToStdin = errors.New("cannot use ‘-w’ flag with standard input")
var errMustWriteToFiles = errors.New("must use ‘-w’ flag with more than one path")

const currentVersion = "1.1.1"

var version = flag.Bool("v", false, "show version")
var write = flag.Bool("w", false, "write results to files")

func main() {
	flag.Usage = usage
	flag.Parse()

	log.SetFlags(0)

	if *version {
		fmt.Println(currentVersion)
		return
	}

	paths := flag.Args()
	if len(paths) == 0 {
		if *write {
			log.Fatal(errCannotWriteToStdin)
		}
		if err := run(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(paths) > 1 && !*write {
		log.Fatal(errMustWriteToFiles)
	}
	for _, p := range paths {
		var in *os.File
		var out **os.File
		var access int
		if *write {
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
		if err := run(in, *out); err != nil {
			log.Fatal(err)
		}
	}
}

// run takes code from in, toggles it, deletes contents of out if it’s in,
// and writes the toggled version to out.
func run(in *os.File, out *os.File) error {
	code, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("run: in io.ReadAll: %v", err)
	}
	toggled, err := toggle(code)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	if out == in {
		if _, err := out.Seek(0, 0); err != nil {
			return fmt.Errorf("run: in *File.Seek: %v", err)
		}
		if err := out.Truncate(0); err != nil {
			return fmt.Errorf("run: in *File.Truncate: %v", err)
		}
	}
	if _, err := out.Write(toggled); err != nil {
		return fmt.Errorf("run: in *File.Write: %v", err)
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

// errorSymbolInfoRegexp catches position and name of the symbol in a build error.
const errorSymbolInfoRegexp = `\d+:\d+: \w+`

var errorSymbolInfo = regexp.MustCompile(errorSymbolInfoRegexp)

var (
	noProviderError = regexp.MustCompile(errorSymbolInfoRegexp + " required module provides package")
	notUsedError    = regexp.MustCompile(errorSymbolInfoRegexp + " declared (but|and) not used")
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
	importsWithoutProviderInfo, err := getSymbolsInfoFromBuildErrors(code, noProviderError)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	var commentedLinesNums []int
	for _, info := range importsWithoutProviderInfo {
		l := &lines[info.lineNum]
		*l = append([]byte(commentPrefix), *l...)
		commentedLinesNums = append(commentedLinesNums, info.lineNum)
	}
	// Check for ‘declared and not used’ errors and create fake usages for them if
	// any.
	notUsedVarsInfo, err := getSymbolsInfoFromBuildErrors(bytes.Join(lines, []byte("\n")), notUsedError)
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

type SymbolInfo struct {
	name    string
	lineNum int
}

// getSymbolsInfoFromBuildErrors tries to build code and checks a build stdout
// for errors catched by r. If any, it returns a slice of tuples with a line
// and a name of every catched symbol.
func getSymbolsInfoFromBuildErrors(code []byte, r *regexp.Regexp) ([]SymbolInfo, error) {
	td, err := os.MkdirTemp(os.TempDir(), "gouse")
	if err != nil {
		return nil, fmt.Errorf("getSymbolsInfoFromBuildErrors: in os.MkdirTemp: %v", err)
	}
	defer os.RemoveAll(td)
	tf, err := os.CreateTemp(td, "*.go")
	if err != nil {
		return nil, fmt.Errorf("getSymbolsInfoFromBuildErrors: in os.CreateTemp: %v", err)
	}
	defer tf.Close()
	tf.Write(code)
	boutput, err := exec.Command("go", "build", "-o", os.DevNull, tf.Name()).CombinedOutput()
	if err == nil {
		return nil, nil
	}
	berrors := strings.Split(string(boutput), "\n")
	var info []SymbolInfo
	for _, e := range berrors {
		if !r.MatchString(e) {
			continue
		}
		symbolInfo := strings.Split(errorSymbolInfo.FindString(e), ":")
		lineNum, err := strconv.Atoi(symbolInfo[0])
		if err != nil {
			return nil, fmt.Errorf("getSymbolsInfoFromBuildErrors: in strconv.Atoi: %v", err)
		}
		info = append(info, SymbolInfo{
			name:    symbolInfo[2],
			lineNum: lineNum - 1, // An adjustment for 0-based count.
		})
	}
	return info, nil
}
