// Command wowfix is the Wails v2 desktop frontend for wowfix. This
// root entrypoint is what `wails build` compiles (wails.json lives at
// the repo root, next to frontend/).
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"

	"github.com/wowfix/wowfix/internal/gui"
)

//go:embed all:frontend/dist
var assets embed.FS

// Build metadata, overridable via -ldflags (see the release workflow).
var version = "dev"
var commit = "none"

func main() {
	if err := wails.Run(gui.App(assets, version)); err != nil {
		println("Error:", err.Error())
	}
}
