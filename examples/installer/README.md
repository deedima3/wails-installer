# Desktop installer example

This example is a separate Wails v2 installer that embeds a compiled application. Its Svelte UI consumes the local component package. A small Wails target application is included in `cmd/lumen`.

## Prerequisites

- Go 1.25 or newer for the pinned Wails v2.15.0 example.
- Node.js 22.12 or newer.
- The Wails v2 CLI and [native build dependencies](https://v2.wails.io/docs/gettingstarted/installation/).

The Go core itself only needs Go 1.22. The example uses `replace github.com/deedima3/wails-installer => ../..`, so keep this directory inside the repository.

## Build the UI

From the repository root:

```sh
npm ci
npm run build
cd examples/installer
go mod download
```

## Prepare a payload

To prepare and package the included sample automatically on Linux with WebKitGTK 4.1, Windows, or macOS, run these commands from the repository root:

```sh
npm run prepare:example
npm run check:bindings
npm run build:example
```

This selects the host platform's executable name and creates the macOS sample bundle's `Info.plist`. The equivalent manual steps follow.

To use your own app, copy its compiled binary and any resources into `payload/`, then update identity and `Executable` in `main.go`. To try the included sample, compile it on the target OS.

Linux with WebKitGTK 4.1:

```sh
go build -tags 'desktop,production,webkit2_41' -o payload/lumen ./cmd/lumen
wails dev -tags webkit2_41
```

For systems with WebKitGTK 4.0, omit `webkit2_41` from both commands.

Windows, in PowerShell:

```powershell
go build -tags "desktop,production" -ldflags "-H=windowsgui" -o payload/lumen.exe ./cmd/lumen
wails dev
```

macOS:

```sh
mkdir -p payload/Lumen.app/Contents/MacOS
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags 'desktop,production' -o payload/Lumen.app/Contents/MacOS/lumen ./cmd/lumen
```

For a complete macOS bundle, copy the `Info.plist` below into `payload/Lumen.app/Contents/Info.plist`, then run `CGO_LDFLAGS="-framework UniformTypeIdentifiers" wails dev`. The explicit framework works around a missing link flag in the pinned Wails v2.15.0 release with current SDKs; the root helper scripts add it automatically. For distribution, use your target application's bundle produced by `wails build`, with its icons and signing workflow.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleExecutable</key><string>lumen</string>
  <key>CFBundleIdentifier</key><string>io.example.lumen</string>
  <key>CFBundleName</key><string>Lumen</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
```

The `payload/README.txt` placeholder can be removed after preparing the payload. Do not commit compiled application binaries. Build the installer separately for every OS and architecture.

## Build for distribution

From this directory:

```sh
wails build
# Linux with WebKitGTK 4.1:
wails build -tags webkit2_41
```

Distribute the resulting installer from `build/bin`. Test the installed target on a clean machine, including its native webview dependencies. Signing, notarization, and prerequisite installation are separate release steps.

## Try installing

Choose a new directory in a writable parent. Leave startup unchecked for an isolated test, or enable it to test the current user's real launch-at-login behavior. After installation, click Open Lumen. The sample application displays a welcome screen.

To upgrade, change `Version` and the payload, rebuild the installer, close Lumen, and select the same installation path. To uninstall, close Lumen, open the installer, choose that path, select Uninstall application, and confirm removal. Personal and modified files are retained. See the [lifecycle guide](../../docs/lifecycle.md).

This example creates a real user login entry when selected. Uninstall removes the owned entry. If manual recovery is necessary, inspect only the `io.example.lumen` entry created by this example:

| Platform | Startup entry |
| --- | --- |
| Windows | `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\io.example.lumen.lnk` |
| macOS | `~/Library/LaunchAgents/io.example.lumen.plist` |
| Linux | `$XDG_CONFIG_HOME/autostart/io.example.lumen.desktop`, defaulting to `~/.config/autostart/io.example.lumen.desktop` |

`Install` refuses repeated fresh installation into an existing directory. `Upgrade` and `Uninstall` require the ownership receipt written by this library. Earlier installations without receipts need the migration described in the lifecycle guide.
