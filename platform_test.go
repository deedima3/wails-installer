package installer

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopEntryEscapesArguments(t *testing.T) {
	entry := desktopEntry("Test\nInjected=true", "/home/me/My app/$cash`quote\"%file\\app")
	if !strings.Contains(entry, `Name=Test\nInjected=true`) {
		t.Fatalf("name can inject desktop keys: %s", entry)
	}
	for _, escaped := range []string{`\\$`, `\\` + "`", `\\"`, `%%file`, `\\\\app`} {
		if !strings.Contains(entry, escaped) {
			t.Fatalf("missing %q in %s", escaped, entry)
		}
	}
}

func TestLaunchAgentUsesOneEscapedExecutableArgument(t *testing.T) {
	path := `/Applications/My & <test>/app`
	document := launchAgent("io.example.test", path)
	decoder := xml.NewDecoder(strings.NewReader(document))
	var values []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "string" {
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				t.Fatal(err)
			}
			values = append(values, value)
		}
	}
	if len(values) != 2 || values[1] != path {
		t.Fatalf("bad plist arguments: %v", values)
	}
	if powershellString("app's $path") != "'app''s $path'" {
		t.Fatal("unsafe PowerShell quoting")
	}
}

func TestNativeStartupDoesNotOverwriteAndCanDisable(t *testing.T) {
	isolateUserDirectories(t)
	info := Info{ID: "io.example.test", Name: "Test"}
	manager := nativeStartup{}
	executable := filepath.Join(t.TempDir(), "Test app's $data Café", "app.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), info, executable); err != nil {
		t.Fatal(err)
	}
	path, err := startupPath(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		verifyWindowsShortcut(t, path, executable)
	}
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("/usr/bin/plutil", "-lint", path).CombinedOutput(); err != nil {
			t.Fatalf("invalid LaunchAgent: %v: %s", err, output)
		}
	}
	if err := manager.Enable(context.Background(), info, filepath.Join(filepath.Dir(executable), "other.exe")); err == nil {
		t.Fatal("overwrote existing startup registration")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("existing startup registration changed")
	}
	if err := manager.Disable(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("startup registration remains")
	}
	if err := manager.Disable(context.Background(), info); err != nil {
		t.Fatal("disable is not idempotent:", err)
	}
}

func TestRealStartupFailurePreservesRegistrationAndRollsBack(t *testing.T) {
	isolateUserDirectories(t)
	c := testConfig(t)
	i := engine(t, c)
	path, _ := startupPath(c.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing registration"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(context.Background(), Request{c.DefaultDirectory, true}, nil); err == nil {
		t.Fatal("expected registration conflict")
	}
	assertNoInstallArtifacts(t, c.DefaultDirectory)
	data, _ := os.ReadFile(path)
	if string(data) != "existing registration" {
		t.Fatal("deleted another registration")
	}
}

func isolateUserDirectories(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
}

func verifyWindowsShortcut(t *testing.T, path, executable string) {
	t.Helper()
	script := "[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false); $s=(New-Object -ComObject WScript.Shell).CreateShortcut(" + powershellString(path) + "); @{target=$s.TargetPath; directory=$s.WorkingDirectory; arguments=$s.Arguments} | ConvertTo-Json -Compress"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("read shortcut: %v: %s", err, output)
	}
	var link struct {
		Target    string
		Directory string
		Arguments string
	}
	if err := json.Unmarshal(output, &link); err != nil {
		t.Fatalf("decode shortcut: %v: %s", err, output)
	}
	if !sameFilesystemPath(link.Target, executable) || !sameFilesystemPath(link.Directory, filepath.Dir(executable)) || link.Arguments != "" {
		t.Fatalf("incorrect shortcut: %+v, expected %s", link, executable)
	}
}

func sameFilesystemPath(first, second string) bool {
	a, err := os.Stat(first)
	if err != nil {
		return false
	}
	b, err := os.Stat(second)
	return err == nil && os.SameFile(a, b)
}

func TestPathIdentityAcceptsFilesystemAliases(t *testing.T) {
	dir := t.TempDir()
	original, alias := filepath.Join(dir, "long-name"), filepath.Join(dir, "alias")
	if err := os.WriteFile(original, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, alias); err != nil {
		t.Skip("hard links unavailable:", err)
	}
	if !sameFilesystemPath(alias, original) {
		t.Fatal("different names for the same file rejected")
	}
	if sameFilesystemPath(filepath.Join(dir, "missing"), original) {
		t.Fatal("missing path accepted")
	}
}

func TestMacBundleLaunchAgent(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "My app's & notes.app")
	executable := filepath.Join(bundle, "Contents", "MacOS", "app")
	if got := macBundle(executable); got != bundle {
		t.Fatalf("bundle: %q", got)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "app"), filepath.Join(bundle, "Resources", "app"), "relative.app/Contents/MacOS/app"} {
		if got := macBundle(path); got != "" {
			t.Fatalf("false bundle: %s -> %s", path, got)
		}
	}
	decoder := xml.NewDecoder(strings.NewReader(launchAgent("io.example.test", executable)))
	var values []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "string" {
			var value string
			if err := decoder.DecodeElement(&value, &element); err != nil {
				t.Fatal(err)
			}
			values = append(values, value)
		}
	}
	if want := []string{"io.example.test", "/usr/bin/open", "-g", bundle}; !reflect.DeepEqual(values, want) {
		t.Fatalf("LaunchAgent arguments: %v, want %v", values, want)
	}
}

func TestDefaultDirectoriesAreNative(t *testing.T) {
	isolateUserDirectories(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	var wantInstall, wantStartup string
	switch runtime.GOOS {
	case "linux":
		wantInstall = filepath.Join(home, ".local", "share", "io.example.test")
		wantStartup = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autostart", "io.example.test.desktop")
	case "windows":
		wantInstall = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "io.example.test")
		wantStartup = filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "io.example.test.lnk")
	case "darwin":
		wantInstall = filepath.Join(home, "Applications", "io.example.test")
		wantStartup = filepath.Join(home, "Library", "LaunchAgents", "io.example.test.plist")
	default:
		t.Skip("unsupported desktop OS")
	}
	if got, err := defaultDirectory("io.example.test"); err != nil || got != wantInstall {
		t.Fatalf("default install path %q, %v, want %q", got, err, wantInstall)
	}
	if got, err := startupPath("io.example.test"); err != nil || got != wantStartup {
		t.Fatalf("startup path %q, %v, want %q", got, err, wantStartup)
	}
}
