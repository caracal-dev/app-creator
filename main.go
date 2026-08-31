package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/caracal-dev/app-creator/internal/guiapp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

func main() {
	frontend, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	app := guiapp.New()
	if err := wails.Run(&options.App{
		Title:            "App Creator",
		Width:            1000,
		Height:           700,
		MinWidth:         800,
		MinHeight:        600,
		BackgroundColour: options.NewRGBA(24, 22, 22, 255),
		AssetServer: &assetserver.Options{
			Assets: frontend,
		},
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
	}); err != nil {
		log.Fatal(err)
	}
}