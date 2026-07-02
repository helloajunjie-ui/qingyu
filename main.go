package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	win := GetSettings().Window

	err := wails.Run(&options.App{
		Title:       win.Title,
		Width:       win.Width,
		Height:      win.Height,
		MinWidth:    win.MinWidth,
		MinHeight:   win.MinHeight,
		AlwaysOnTop: win.AlwaysOnTop,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        app.startup,
		Windows: &windows.Options{
			WebviewIsTransparent: win.Transparent,
			WindowIsTranslucent:  win.Transparent,
			DisableWindowIcon:    win.DisableIcon,
		},
		Frameless: win.Frameless,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
