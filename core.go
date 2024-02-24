package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const commentPrefix = "// "

const (
	symbolInfoInErrorRegexp = `\d+:\d+: \w+`

	fakeUsagePrefix = "; _ ="
	fakeUsageSuffix = " /* TODO: gouse */"
)

var (
	escapedFakeUsageSuffix = regexp.QuoteMeta(fakeUsageSuffix)
	fakeUsage              = regexp.MustCompile(fakeUsagePrefix + ".*" + escapedFakeUsageSuffix)
	fakeUsageAfterGofmt    = regexp.MustCompile(`\s*_\s*= \w*\s*` + escapedFakeUsageSuffix)

	noProviderError = regexp.MustCompile(symbolInfoInErrorRegexp + " required module provides package")
	notUsedError    = regexp.MustCompile(symbolInfoInErrorRegexp + " declared (but|and) not used")
)

// toggle returns toggled code. First it tries to remove previosly created fake
// usages. If there is nothing to remove, it creates them.
func toggle(ctx context.Context, code []byte) ([]byte, error) {
	// fakeUsage must be before fakeUsageAfterGofmt because it also removes the leading ‘;’.
	if fakeUsage.Match(code) {
		return fakeUsage.ReplaceAll(code, []byte("")), nil
	}
	if fakeUsageAfterGofmt.Match(code) {
		return fakeUsageAfterGofmt.ReplaceAll(code, []byte("")), nil
	}

	lines := bytes.Split(code, []byte("\n"))
	// Check for problematic imports and comment them out if any, storing commented
	// out lines numbers to commentedLinesNums.
	importsWithoutProviderInfo, err := getSymbolsInfoFromBuildErrors(ctx, code, noProviderError)
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
	notUsedVarsInfo, err := getSymbolsInfoFromBuildErrors(
		ctx,
		bytes.Join(lines, []byte("\n")),
		notUsedError,
	)
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

// symbolInfo represents name and line number of symbols (variables, functions,
// imports, etc.) from build errors.
type symbolInfo struct {
	name    string
	lineNum int
}

var symbolInfoInError = regexp.MustCompile(symbolInfoInErrorRegexp)

// getSymbolsInfoFromBuildErrors tries to build code and checks a build stdout
// for errors catched by r. If any, it returns a slice of structs with a line
// and a name of every catched symbol.
func getSymbolsInfoFromBuildErrors(ctx context.Context, code []byte, r *regexp.Regexp) ([]symbolInfo, error) {
	select {
	case <-ctx.Done():
		return nil, nil
	default:
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
		var info []symbolInfo
		for _, e := range berrors {
			if !r.MatchString(e) {
				continue
			}
			symInfo := strings.Split(symbolInfoInError.FindString(e), ":")
			lineNum, err := strconv.Atoi(symInfo[0])
			if err != nil {
				return nil, fmt.Errorf("getSymbolsInfoFromBuildErrors: in strconv.Atoi: %v", err)
			}
			info = append(info, symbolInfo{
				name:    symInfo[2],
				lineNum: lineNum - 1, // An adjustment for 0-based count.
			})
		}
		return info, nil
	}
}
