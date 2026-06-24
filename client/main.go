package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	_ "github.com/joho/godotenv/autoload"

	"github.com/neev/remote-agent/client/backend"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	app := backend.NewApp()
	backend.InstallLogger(app)

	err := wails.Run(&options.App{
		Title:     "Neev Remote",
		Width:     920,
		Height:    680,
		MinWidth:  700,
		MinHeight: 500,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 240, G: 242, B: 245, A: 255},
		OnStartup:        app.Startup,
		OnDomReady:       app.DomReady,
		OnShutdown:       app.Shutdown,
		Bind: []interface{}{
			app,
		},
		// macOS options
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		// Windows options
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableFramelessWindowDecorations: false,
		},
		// Frameless for custom title bar
		Frameless: false,
	})

	if err != nil {
		panic(err)
	}
}
