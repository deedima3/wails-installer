package consumer_test

import (
	"context"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	installer "github.com/deedima3/wails-installer"
)

func TestImportedServiceInstallsRegistersAndLaunches(t *testing.T) {
	// This is a separate module. It can only use the public Go API.
	source := t.TempDir()
	executable := "bin/probe"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", filepath.Join(source, filepath.FromSlash(executable)), "./testdata/probe")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real payload: %v: %s", err, output)
	}

	// Never modify the developer's actual login settings.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	destination := filepath.Join(t.TempDir(), "My app's & $data Café")
	engine, err := installer.New(installer.Config{
		ID: "io.example.consumer", Name: "Consumer app", Version: "1.0.0",
		Payload: os.DirFS(source), Executable: executable,
		DefaultDirectory: destination, AllowStartup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := installer.NewService(engine, func() (string, error) { return destination, nil })
	if got, err := service.Browse(); err != nil || got != destination {
		t.Fatalf("picker: %q %v", got, err)
	}
	result, err := service.Install(installer.Request{Directory: destination, Startup: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Startup || service.State().Status != "complete" {
		t.Fatalf("installation did not complete: %+v", service.State())
	}
	if _, err := os.Stat(result.Executable); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	pid := waitForLaunch(t, result.Executable)
	if runtime.GOOS == "windows" {
		link := filepath.Join(home, "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "io.example.consumer.lnk")
		marker := filepath.Join(filepath.Dir(result.Executable), "launched.txt")
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		// Follow the actual .lnk through ShellExecute, just as the Startup folder
		// does. Wait for the first probe before reusing its marker.
		script := "$ErrorActionPreference='Stop'; $p=Get-Process -Id " + strconv.Itoa(pid) + " -ErrorAction SilentlyContinue; if ($p) {$p.WaitForExit()}; Remove-Item -LiteralPath " + quote(marker) + "; Start-Process -FilePath " + quote(link) + " -Wait"
		command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("launch startup shortcut: %v: %s", err, output)
		}
		pid = waitForLaunch(t, result.Executable)
	}
	// The same public API supports maintenance after the installer is restarted.
	if managed, err := service.Inspect(destination); err != nil || managed == nil || managed.Version != "1.0.0" {
		t.Fatalf("inspect installed application: %+v %v", managed, err)
	}
	if runtime.GOOS == "darwin" {
		waitForMacExit(t, pid)
	}
	// Do not run upgrade against a still-running executable on Windows.
	if runtime.GOOS == "windows" {
		command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$p=Get-Process -Id "+strconv.Itoa(pid)+" -ErrorAction SilentlyContinue; if($p){$p.WaitForExit()}; exit 0")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("wait for probe: %v: %s", err, output)
		}
	}
	if err := os.Remove(filepath.Join(filepath.Dir(result.Executable), "launched.txt")); err != nil {
		t.Fatal(err)
	}
	writePersonal := filepath.Join(destination, "notes.txt")
	if err := os.WriteFile(writePersonal, []byte("keep my notes"), 0600); err != nil {
		t.Fatal(err)
	}
	newEngine, err := installer.New(installer.Config{ID: "io.example.consumer", Name: "Consumer app", Version: "2.0.0", Payload: os.DirFS(source), Executable: executable, DefaultDirectory: destination, AllowStartup: true})
	if err != nil {
		t.Fatal(err)
	}
	maintenance := installer.NewService(newEngine, nil)
	if _, err := maintenance.Upgrade(installer.Request{Directory: destination, Startup: true}); err != nil {
		t.Fatal(err)
	}
	if managed, err := maintenance.Inspect(destination); err != nil || managed == nil || managed.Version != "2.0.0" {
		t.Fatalf("inspect upgrade: %+v %v", managed, err)
	}
	removed, err := maintenance.Uninstall(destination)
	if err != nil {
		t.Fatal(err)
	}
	if removed.DirectoryRemoved || len(removed.PreservedFiles) != 1 || removed.PreservedFiles[0] != "notes.txt" {
		t.Fatalf("uninstall did not preserve personal files: %+v", removed)
	}
	if data, err := os.ReadFile(writePersonal); err != nil || string(data) != "keep my notes" {
		t.Fatalf("personal data changed: %q %v", data, err)
	}
	if _, err := os.Stat(result.Executable); !os.IsNotExist(err) {
		t.Fatalf("application was not removed: %v", err)
	}
}

func waitForLaunch(t *testing.T, executable string) int {
	t.Helper()
	marker := filepath.Join(filepath.Dir(executable), "launched.txt")
	deadline := time.After(15 * time.Second)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("installed executable never ran")
		case <-tick.C:
			if data, err := os.ReadFile(marker); err == nil && strings.HasPrefix(string(data), "launched ") {
				pid, err := strconv.Atoi(strings.TrimPrefix(string(data), "launched "))
				if err != nil {
					t.Fatal(err)
				}
				return pid
			}
		}
	}
}

func TestImportedMacBundleLaunchAndLoginCommand(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("LaunchServices is only available on macOS")
	}
	source := t.TempDir()
	executable := "Consumer.app/Contents/MacOS/probe"
	target := filepath.Join(source, filepath.FromSlash(executable))
	build := exec.Command("go", "build", "-buildvcs=false", "-o", target, "./testdata/probe")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bundle payload: %v: %s", err, output)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>probe</string><key>CFBundleIdentifier</key><string>io.example.consumer.bundle</string><key>CFBundlePackageType</key><string>APPL</string><key>LSUIElement</key><true/></dict></plist>`
	if err := os.WriteFile(filepath.Join(source, "Consumer.app", "Contents", "Info.plist"), []byte(plist), 0644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	engine, err := installer.New(installer.Config{
		ID: "io.example.consumer.bundle", Name: "Consumer bundle", Version: "1",
		Payload: os.DirFS(source), Executable: executable, AllowStartup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := installer.NewService(engine, nil)
	result, err := service.Install(installer.Request{Directory: filepath.Join(t.TempDir(), "My app's & notes"), Startup: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	waitForMacExit(t, waitForLaunch(t, result.Executable))
	marker := filepath.Join(filepath.Dir(result.Executable), "launched.txt")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	// Exercise the installed LaunchAgent command without registering or loading
	// a job in the actual user's launchd session.
	data, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", "io.example.consumer.bundle.plist"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var arguments []string
	for {
		token, err := decoder.Token()
		if err != nil {
			t.Fatal("missing ProgramArguments:", err)
		}
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "array" {
			var array struct {
				Values []string `xml:"string"`
			}
			if err := decoder.DecodeElement(&array, &element); err != nil {
				t.Fatal(err)
			}
			arguments = array.Values
			break
		}
	}
	if len(arguments) != 3 || arguments[0] != "/usr/bin/open" {
		t.Fatalf("unexpected LaunchAgent command: %v", arguments)
	}
	if output, err := exec.Command(arguments[0], arguments[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("run LaunchAgent command: %v: %s", err, output)
	}
	waitForMacExit(t, waitForLaunch(t, result.Executable))
}

func waitForMacExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("probe did not exit")
		case <-tick.C:
			if err := exec.Command("/bin/kill", "-0", strconv.Itoa(pid)).Run(); err != nil {
				return
			}
		}
	}
}
