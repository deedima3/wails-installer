# Platform support and validation

The Go engine supports per-user installs, upgrades, and uninstalling on Linux, Windows, and macOS. It has no Wails dependency, so an existing Wails project can import it and bind `Service`. The optional Svelte component accepts generated bindings through `createClient`.

## Platform behavior

| Platform | Payload entrypoint | Default installation | Launch at login | Launch action |
| --- | --- | --- | --- | --- |
| Linux | Regular executable, such as `myapp` | `~/.local/share/<id>` | XDG autostart `.desktop` file | Start the executable directly |
| Windows | Regular executable, such as `myapp.exe` | `%LOCALAPPDATA%\Programs\<id>` | Current user's Startup folder `.lnk` | Start the executable directly |
| macOS | `<name>.app/Contents/MacOS/<executable>` | `~/Applications/<id>` | User LaunchAgent invoking `open -g` | Open the `.app` through LaunchServices |

Standalone macOS binaries also work, using direct process launching and a LaunchAgent pointing to the binary. Payload files must be regular files, without symlinks. Build the target and installer for the same OS and architecture. macOS applications with symlinked frameworks need a different payload strategy.

## Automated coverage

The [workflow](../.github/workflows/ci.yml) is configured with native Ubuntu, Windows, and macOS runners for both engine tests and desktop builds. It does not emulate Windows or macOS on Linux.

| Check | Linux | Windows | macOS |
| --- | --- | --- | --- |
| Copy, cancellation, rollback, path validation, concurrent installs | Native race tests | Native race tests | Native race tests |
| Upgrade, data preservation, modified-file protection, rollback, uninstall | Native race tests | Native race tests | Native race tests |
| Native default install and startup directories | Native test | Native test | Native test |
| Create, inspect, preserve, and remove startup entries in temporary user directories | `.desktop` file | Actual `.lnk`, read through WScript.Shell | LaunchAgent, checked with `plutil` |
| Independent module imports the library, installs/launches/upgrades/uninstalls, and preserves notes | Native test | Native test | Native test |
| Run the generated native login command | Registration checked | Open the `.lnk` through the OS shell | Run the `.app` LaunchAgent command |
| Install and open a conventional `.app` bundle | Not applicable | Not applicable | Native LaunchServices test |
| Generate Wails v2 bindings and type-check `createClient` against them | Native build job | Native build job | Native build job |
| Build the target app and package the importing Wails installer | Native build job | Native build job | Native build job |

The table describes the checks configured in CI, not a claim that a particular remote workflow run has passed. In the current Linux development environment, engine race tests, the independent consumer's install/startup/launch test, generated binding checks, and the full Linux Wails build pass. The library's test binaries also cross-compile for amd64 and arm64 on Linux, Windows, and macOS. Native Windows and macOS results must come from those CI runners or actual target machines.

## Reproduce locally

Run from the repository root, with Go 1.25+, Node.js 22.12+, the Wails v2 CLI, and the platform's native Wails prerequisites installed:

```sh
go test -race -cover ./...
go vet ./...
go -C integration/consumer test -race -v ./...
npm ci
npm run build
npm run prepare:example
npm run check:bindings
npm run build:example
```

`prepare:example` builds the payload for the current host OS. On Linux these scripts select WebKitGTK 4.1. For WebKitGTK 4.0 use the manual commands in the [example guide](../examples/installer/README.md).

The root Go module requires Go 1.22+, while the pinned Wails example requires Go 1.25+. The independent consumer uses only the public Go API and a local `replace` directive. It does not depend on a published module tag, npm package, or Wails installation.

## Release checks

Automated tests verify registration files and launch commands without changing real login settings. They do not log a user out and back in. Before distributing a signed release, test installation and actual login behavior on a clean target machine, including any WebView2 or WebKitGTK prerequisites. macOS signing and notarization, Windows signing, and system-wide installation policies belong to the application's release process.

See [integration](integration.md) for import instructions, [customization](customization.md) for extension interfaces, and [API reference](api.md) for the service contract. Fetch the Go module from this repository's `main` branch, or use a local replacement for development. Install the Svelte package through a local checkout or archive until it is published to npm.
