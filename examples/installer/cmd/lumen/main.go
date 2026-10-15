// A small Wails application to use as an example payload.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed index.html
var assets embed.FS

func main() {
	if err := wails.Run(&options.App{Title: "Lumen", Width: 800, Height: 560, AssetServer: &assetserver.Options{Assets: assets}}); err != nil {
		log.Fatal(err)
	}
}
