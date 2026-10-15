# Independent Go consumer test

This directory has its own `go.mod` and imports the installer through its public module path. It cannot use unexported implementation details.

```sh
cd integration/consumer
go test -race -v ./...
```

The test compiles a small executable for the host OS, installs it into a path containing spaces, Unicode, and shell metacharacters, registers launch at login in an isolated temporary user directory, and launches the installed executable. It verifies that the process actually ran by reading a marker file. On Windows it also opens the generated `.lnk` through the OS shell. It then restarts the service with a new version, upgrades the installation, uninstalls it, and checks that personal notes survive. No real user startup entry is changed.

On macOS, an additional native test creates and installs a `.app`, opens it through the default launcher, then runs the generated LaunchAgent command. It confirms execution both times without loading a launchd job. That test is skipped on other systems because they do not provide LaunchServices.

CI is configured to run these tests on Linux, Windows, and macOS. The separate desktop CI job builds a real Wails project that imports the same library and checks its generated frontend bindings.
