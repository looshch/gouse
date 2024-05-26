# Test Go Files
These files are fed to `gouse` to test it. Each file describes what it tests in
godoc-like comments for `main()`. All their relations are described below.
* * `not_used.{input|golden}` and `used.{input|golden}` cover every first and
    every second processing of the same file correspondingly. They test the
    general use of `gouse`.
  * `not_used_{no_provider|var_and_import}.{input|golden}` test cases when
    an import is either unused or missing.
  * `used_gofmted{|_different_name_length}.{input|golden}` checks cases when
    files are `gofmt`ed after creating fake usages.
