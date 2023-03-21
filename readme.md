# gouse
Toggle ‘declared and not used’ errors in Go by using idiomatic `_ = notUsedVar`
and leaving a TODO comment. ![a demo](demo.gif)

## Installation
```
go install github.com/looshch/gouse@latest
```

## Usage
By default, gouse accepts code from stdin or from a file provided as a path
argument and writes the toggled version to stdout. ‘-w’ flag writes the result
back to the file. If multiple paths provided, gouse writes results back to them
regardless of ‘-w’ flag.


### Examples
```
$ gouse
...input...
notUsed = true
...input...

...output...
notUsed = true; _ = notUsed /* TODO: gouse */
...output...
```
```
$ gouse main.go
...
notUsed = true; _ = notUsed /* TODO: gouse */
...
```
```
$ gouse main.go io.go core.go
$ cat main.go io.go core.go
...
notUsedFromMain = true; _ = notUsedFromMain /* TODO: gouse */
...
notUsedFromIo = true; _ = notUsedFromIo /* TODO: gouse */
...
notUsedFromCore = true; _ = notUsedFromCore /* TODO: gouse */
...
```

## How it works
First it tries to remove previously created fake usages. If there is nothing to
remove, it tries to build an input and checks the build stdout for ‘declared
and not used’ errors. If there is any, it creates fake usages for unused
variables from the errors.

## [Why](https://loosh.ch/blog/gouse)
TL; DR: to automate automatable.

## Integrations
* Vim: just bind `<cmd> w <bar> !gouse -w %<cr><cr>` to some mapping.
* [Visual Studio Code plugin](https://github.com/looshch/gouse-vsc).

## Credits
Inspired by [Nikita Rabaev](https://github.com/nikrabaev)’s idea.
