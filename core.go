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

const (
	fakeUsageSuffix = " /* TODO: gouse */"
	fakeUsagePrefix = "; _ ="

	noProviderErrorRegexpSuffix = "no required module provides package"
	commentPrefix               = "// "

	notUsedErrorRegexpSuffix = "declared and not used:"
)

var (
	escapedFakeUsageSuffix = regexp.QuoteMeta(fakeUsageSuffix)
	fakeUsage              = regexp.MustCompile(
		fakeUsagePrefix + ".*" + escapedFakeUsageSuffix,
	)
	fakeUsageAfterGofmt = regexp.MustCompile(
		`\s*_\s*= \w*\s*` + escapedFakeUsageSuffix,
	)
)

// toggle returns toggled code. First it tries to remove previosly created fake
// usages. If there is nothing to remove, it creates them.
func toggle(ctx context.Context, code []byte) ([]byte, error) {
	// fakeUsage must be before fakeUsageAfterGofmt because it also removes
	// the leading ‘;’.
	if fakeUsage.Match(code) {
		return fakeUsage.ReplaceAll(code, []byte("")), nil
	}
	if fakeUsageAfterGofmt.Match(code) {
		return fakeUsageAfterGofmt.ReplaceAll(code, []byte("")), nil
	}

	lines := bytes.Split(code, []byte("\n"))
	// Check for problematic imports and comment them out if any, storing
	// commented out lines numbers to commentedLinesNums.
	importsWithoutProviderInfo, err := getSymbolsInfoFromBuildErrors(
		ctx, code, noProviderErrorRegexpSuffix,
	)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	var commentedLinesNums []int
	for _, info := range importsWithoutProviderInfo {
		l := &lines[info.lineNum]
		*l = append([]byte(commentPrefix), *l...)
		commentedLinesNums = append(commentedLinesNums, info.lineNum)
	}
	// Check for ‘declared and not used’ errors and create fake usages for
	// them if any.
	notUsedVarsInfo, err := getSymbolsInfoFromBuildErrors(
		ctx,
		bytes.Join(lines, []byte("\n")),
		notUsedErrorRegexpSuffix,
	)
	if err != nil {
		return nil, fmt.Errorf("toggle: %v", err)
	}
	for _, info := range notUsedVarsInfo {
		l := &lines[info.lineNum]
		*l = append(*l, []byte(
			fakeUsagePrefix+info.name+fakeUsageSuffix)...,
		)
	}
	// Un-comment commented out lines.
	for _, line := range commentedLinesNums {
		l := &lines[line]
		uncommentedLine := []rune(
			string(*l),
		)[len([]rune(commentPrefix)):]
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

const (
	goFileExt    = ".go"
	nameIndex    = 1
	lineNumIndex = 1
)

var (
	// symbolPositionInErrorRegexp catches the Go file extension and the
	// position of the symbol from the error with the trailing space
	// symbol.
	//
	// Example
	//
	//	Given a build error ‘.../main[.go:4:2: ]<text of an error>’,
	//	the catch group is denoted with ‘[]’.
	symbolPositionInErrorRegexp = regexp.QuoteMeta(goFileExt) +
		`:\d+:\d+: `
	symbolPositionInError = regexp.MustCompile(
		symbolPositionInErrorRegexp,
	)
)

// getSymbolsInfoFromBuildErrors tries to build code and checks a build stdout
// for errors catched by r. If any, it returns a slice of structs with a line
// and a name of every catched symbol.
func getSymbolsInfoFromBuildErrors(
	ctx context.Context, code []byte, suffix string,
) ([]symbolInfo, error) {
	select {
	case <-ctx.Done():
		return nil, nil
	default:
		const thisName = "getSymbolsInfoFromBuildErrors"

		td, err := os.MkdirTemp(os.TempDir(), "gouse")
		if err != nil {
			format := thisName + ": in os.MkdirTemp: %v"
			return nil, fmt.Errorf(format, err)
		}
		defer os.RemoveAll(td)
		tf, err := os.CreateTemp(td, "*"+goFileExt)
		if err != nil {
			format := thisName + ": in os.CreateTemp: %v"
			return nil, fmt.Errorf(format, err)
		}
		defer tf.Close()
		tf.Write(code)
		boutput, err := exec.Command(
			"go", "build", "-o", os.DevNull, tf.Name(),
		).CombinedOutput()
		if err == nil {
			return nil, nil
		}
		berrors := strings.Split(string(boutput), "\n")
		var info []symbolInfo
		r := regexp.MustCompile(symbolPositionInErrorRegexp + suffix)
		for _, e := range berrors {
			if !r.MatchString(e) {
				continue
			}
			lineNum, err := strconv.Atoi(strings.Split(
				symbolPositionInError.FindString(e), ":",
			)[lineNumIndex])
			if err != nil {
				format := thisName + ": in strconv.Atoi: %v"
				return nil, fmt.Errorf(format, err)
			}
			info = append(info, symbolInfo{
				name: strings.Split(e, suffix)[nameIndex],
				// -1 is an adjustment for 0-based count.
				lineNum: lineNum - 1,
			})
		}
		return info, nil
	}
}
