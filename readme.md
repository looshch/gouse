[![Go Reference](https://pkg.go.dev/badge/github.com/looshch/gouse.svg)](https://pkg.go.dev/github.com/looshch/gouse)
[![Go Report Card](https://goreportcard.com/badge/github.com/looshch/gouse)](https://goreportcard.com/report/github.com/looshch/gouse)
[![codecov](https://codecov.io/gh/looshch/gouse/branch/master/graph/badge.svg?token=7SDOQ68E2E)](https://codecov.io/gh/looshch/gouse)

# gouse
Toggle ‘declared but not used’ errors in Go by using idiomatic `_ = notUsedVar`
and leaving a TODO comment. ![a demo gif](demo.gif)

## Installation
```
go install github.com/looshch/gouse@latest
```

## Usage
By default, gouse accepts code from stdin and writes a toggled version to
stdout. If any file paths provided, it takes code from them and writes a
toggled version to stdout unless ‘-w’ flag is passed — then it will write
back to the file paths.
### Examples
```
$ gouse
...input...

notUsed = false

...input...
...output...

notUsed = false; _ = notUsed /* TODO: gouse */

...output...
```
```
$ gouse main.go
...output...

notUsed = false; _ = notUsed /* TODO: gouse */

...output...
```
```
$ gouse -w main.go io.go core.go
```

## How it works
First it tries to remove fake usages. If there is nothing to remove, it tries
to build an input and checks a build stdout for the errors. If there is any,
it creates fake usages for unused variables from the errors.

## [Why](https://loosh.ch/blog/gouse)
TL; DR: to automate automatable and speed up feedback loop.

## Integrations
* Vim: just bind `<cmd> w <bar> !gouse -w %<cr><cr>` to some mapping.
* [Visual Studio Code plugin](https://github.com/looshch/gouse-vsc).
### Help Wanted
I have zero willingness to touch Java world to implement a wrapper for GoLand.
If anyone wants to help, I’ll be glad to include a link to your wrapper.

## Credits
Inspired by [Ilya Polyakov](https://github.com/PolyakovIlya)’s idea.
