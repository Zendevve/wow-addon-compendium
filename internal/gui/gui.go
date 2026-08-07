// Package gui assembles the Wails v2 application options shared by the
// entrypoints: the repo-root main.go that `wails build` compiles, and
// cmd/wowfix-gui. The frontend assets are injected so each entrypoint
// embeds its own dist directory.
package gui

import (
	"io/fs"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/wowfix/wowfix/internal/service"
)

// App returns the wails application options with the service bound.
// assets carries the compiled frontend; version seeds the service's
// reported build version.
func App(assets fs.FS, version string) *options.App {
	svc := service.New(nil)
	svc.Version = version
	return &options.App{
		Title:     "wowfix — WoW addon fixer",
		Width:     1100,
		Height:    720,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{
			svc,
		},
	}
}
