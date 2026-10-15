package installer

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type processLauncher struct{}

func (processLauncher) Launch(executable string) error {
	if runtime.GOOS == "darwin" {
		if bundle := macBundle(executable); bundle != "" {
			output, err := exec.Command("/usr/bin/open", bundle).CombinedOutput()
			if err != nil {
				return fmt.Errorf("open application bundle: %w: %s", err, output)
			}
			return nil
		}
	}
	cmd := exec.Command(executable)
	cmd.Dir = filepath.Dir(executable)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// macBundle recognizes the main executable of a conventional application bundle.
// Other executables keep the direct-process launch behavior.
func macBundle(executable string) string {
	macOS := filepath.Dir(executable)
	contents := filepath.Dir(macOS)
	bundle := filepath.Dir(contents)
	if filepath.IsAbs(executable) && filepath.Base(macOS) == "MacOS" && filepath.Base(contents) == "Contents" && strings.EqualFold(filepath.Ext(bundle), ".app") {
		return bundle
	}
	return ""
}

type nativeStartup struct{}

func (nativeStartup) Snapshot(ctx context.Context, info Info) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := startupPath(info.ID)
	if err != nil {
		return nil, err
	}
	stat, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
		return nil, errors.New("startup registration is not a regular file or exceeds size limit")
	}
	return os.ReadFile(name)
}

func (nativeStartup) Restore(ctx context.Context, info Info, data []byte) error {
	if _, err := (nativeStartup{}).Snapshot(ctx, info); err != nil {
		return err
	}
	if data == nil {
		return (nativeStartup{}).Disable(ctx, info)
	}
	name, err := startupPath(info.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".startup-restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), name)
}

func startupPath(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "linux":
		config := os.Getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(config) {
			config = filepath.Join(home, ".config")
		}
		return filepath.Join(config, "autostart", id+".desktop"), nil
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", id+".plist"), nil
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", id+".lnk"), nil
	default:
		return "", fmt.Errorf("launch at login is unsupported on %s", runtime.GOOS)
	}
}

func (nativeStartup) Enable(ctx context.Context, info Info, executable string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := startupPath(info.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return windowsShortcut(ctx, target, executable, info.Name)
	}
	var content string
	if runtime.GOOS == "darwin" {
		content = launchAgent(info.ID, executable)
	} else {
		content = desktopEntry(info.Name, executable)
	}
	return writeExclusive(target, []byte(content))
}

func (nativeStartup) Disable(_ context.Context, info Info) error {
	target, err := startupPath(info.ID)
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func writeExclusive(target string, content []byte) error {
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return errors.Join(err, os.Remove(target))
	}
	return nil
}

func desktopString(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(value)
}

func desktopEntry(name, executable string) string {
	// Escape the Exec argument first, then the desktop-entry string. %% is a
	// literal percent, so paths cannot inject desktop field codes.
	argument := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$", "%", "%%").Replace(executable)
	return "[Desktop Entry]\nType=Application\nName=" + desktopString(name) + "\nExec=" + desktopString("\""+argument+"\"") + "\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
}

func xmlText(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func launchAgent(id, executable string) string {
	arguments := []string{executable}
	if bundle := macBundle(executable); bundle != "" {
		// Let LaunchServices launch the bundle within the user's GUI session.
		arguments = []string{"/usr/bin/open", "-g", bundle}
	}
	var programArguments strings.Builder
	for _, argument := range arguments {
		programArguments.WriteString("<string>" + xmlText(argument) + "</string>")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>` + xmlText(id) + `</string><key>ProgramArguments</key><array>` + programArguments.String() + `</array><key>RunAtLoad</key><true/></dict></plist>
`
}

func powershellString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func windowsShortcut(ctx context.Context, target, executable, name string) error {
	// Generate in a private directory, then copy with exclusive creation.
	// This also works on redirected Startup folders without hard-link support.
	dir, err := os.MkdirTemp(filepath.Dir(target), ".installer-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	temp := filepath.Join(dir, "startup.lnk")
	script := "$ErrorActionPreference='Stop'; $s=(New-Object -ComObject WScript.Shell).CreateShortcut(" + powershellString(temp) + "); $s.TargetPath=" + powershellString(executable) + "; $s.WorkingDirectory=" + powershellString(filepath.Dir(executable)) + "; $s.Description=" + powershellString(name) + "; $s.Save()"
	if output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput(); err != nil {
		return fmt.Errorf("create startup shortcut: %w: %s", err, output)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	content, err := os.ReadFile(temp)
	if err != nil {
		return err
	}
	return writeExclusive(target, content)
}
