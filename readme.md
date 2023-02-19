# gouse
Toggle ‘declared but not used’ errors in Go by using idiomatic `_ = notUsedVar`
and leaving a TODO comment. ![a demo gif](demo.gif)

## Installation
```
go install github.com/looshch/gouse@latest
```

## Usage
By default, gouse accepts code from stdin and writes a toggled version to
stdout. If any file paths provided with ‘-w’ flag, it writes a toggled version
back to them, or to stdout if only one path provided without the flag.

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
...
notUsed = false; _ = notUsed /* TODO: gouse */
...
```
```
$ gouse -w main.go io.go core.go
$ cat main.go io.go core.go
...
notUsedFromMain = false; _ = notUsedFromMain /* TODO: gouse */
...
notUsedFromIo = false; _ = notUsedFromIo /* TODO: gouse */
...
notUsedFromCore = false; _ = notUsedFromCore /* TODO: gouse */
...
```

## How it works
First it tries to remove fake usages. If there is nothing to remove, it tries
to build an input and checks a build stdout for the errors. If there is any,
it creates fake usages for unused variables from the errors.

## [Why](https://loosh.ch/blog/gouse)
TL; DR: to automate automatable.

## Integrations
* Vim: just bind `<cmd> w <bar> !gouse -w %<cr><cr>` to some mapping.
* [Visual Studio Code plugin](https://github.com/looshch/gouse-vsc).

## Credits
Inspired by [Nikita Rabaev](https://github.com/nikrabaev)’s idea.
