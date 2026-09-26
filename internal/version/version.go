// Package version holds the version of the tool. It is its own package so
// that the language can read it without the evaluator depending on the CLI.
package version

// Current is the version reported by "--version" and by the "$yakVersion"
// variable. A build stamps it in with
// -ldflags "-X github.com/daluz/yak/internal/version.Current=1.2.3";
// an unstamped build calls itself "dev".
var Current = "dev"
