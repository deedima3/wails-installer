package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	installer "github.com/deedima3/wails-installer"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Build the target app into payload before building this separate installer.
//
//go:embed all:payload
var payload embed.FS

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	payloadFS, err := fs.Sub(payload, "payload")
	must(err)
	executable := "lumen"
	switch runtime.GOOS {
	case "windows":
		executable = "lumen.exe"
	case "darwin":
		executable = "Lumen.app/Contents/MacOS/lumen"
	}
	engine, err := installer.New(installer.Config{
		ID: "io.example.lumen", Name: "Lumen", Version: "1.0.0",
		Description: "A quieter workspace for notes, plans, and the ideas you want to keep.",
		Payload:     payloadFS, Executable: executable, AllowStartup: true,
	})
	must(err)
	var appContext context.Context
	service := installer.NewService(engine, func() (string, error) {
		parent, err := wailsruntime.OpenDirectoryDialog(appContext, wailsruntime.OpenDialogOptions{Title: "Choose a parent folder for Lumen"})
		if err != nil || parent == "" {
			return "", err
		}
		return filepath.Join(parent, "Lumen"), nil
	})
	ui, err := fs.Sub(assets, "frontend/dist")
	must(err)
	must(wails.Run(&options.App{
		Title: "Install Lumen", Width: 1060, Height: 800, MinWidth: 560, MinHeight: 680,
		AssetServer: &assetserver.Options{Assets: ui},
		OnStartup:   func(ctx context.Context) { appContext = ctx },
		OnBeforeClose: func(context.Context) bool {
			// Keep the window open until cancellation and staging cleanup finish.
			if service.State().Status == "installing" {
				service.Cancel()
				return true
			}
			return false
		},
		OnShutdown: func(context.Context) { service.Cancel() },
		Bind:       []interface{}{service},
	}))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
