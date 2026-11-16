package main

import (
	"os"
	"runtime"

	"github.com/songjiarong/mtok/internal/app"
)

// Build metadata, injected by scripts/build.ps1.
var (
	Version = "0.2.1"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	code := app.Run(app.Args{
		Argv:      os.Args[1:],
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		IsTTY:     app.IsTerminal(os.Stdout),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		GoVersion: runtime.Version(),
	})
	os.Exit(code)
}
