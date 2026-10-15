// Package installer copies an application payload to a user-selected directory.
// It has no Wails dependency. Bind Service in Wails or use Installer directly.
package installer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

// Config describes one trusted, platform-specific application payload.
type Config struct {
	ID          string
	Name        string
	Version     string
	Description string
	Payload     fs.FS
	// Executable is an fs.ValidPath relative to Payload, including a .app's
	// Contents/MacOS path on macOS. It is always given executable permissions.
	Executable  string
	Executables []string
	// PreservePaths are payload-relative files or directory trees retained across
	// upgrades and uninstall. Keep mutable application data under these paths.
	PreservePaths    []string
	DefaultDirectory string
	AllowStartup     bool
	// MaxBytes caps the extracted payload. Zero uses 2 GiB.
	MaxBytes int64
	Hooks    Hooks
}

// Hooks run before publication. Write generated files only inside StageDirectory.
// Hooks must honor ctx and undo any external side effects themselves.
type Hooks struct {
	BeforeCopy      func(context.Context, Installation) error
	AfterCopy       func(context.Context, Installation) error
	BeforeUninstall func(context.Context, Installation) error
}

type Installation struct {
	StageDirectory string
	Directory      string
	Executable     string
}

type Request struct {
	Directory string `json:"directory"`
	Startup   bool   `json:"startup"`
}

type Result struct {
	Directory  string   `json:"directory"`
	Executable string   `json:"executable"`
	Startup    bool     `json:"startup"`
	Warnings   []string `json:"warnings"`
}

type ManagedInstallation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Directory  string `json:"directory"`
	Executable string `json:"executable"`
	Startup    bool   `json:"startup"`
}

type UninstallResult struct {
	Directory        string   `json:"directory"`
	RemovedFiles     int      `json:"removedFiles"`
	PreservedFiles   []string `json:"preservedFiles"`
	DirectoryRemoved bool     `json:"directoryRemoved"`
	Warnings         []string `json:"warnings"`
}

type Info struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	Description      string `json:"description"`
	DefaultDirectory string `json:"defaultDirectory"`
	AllowStartup     bool   `json:"allowStartup"`
	Platform         string `json:"platform"`
}

// StartupManager registers the installed executable for the current user's login.
// Enable must leave the old registration untouched on failure. Disable reverses
// a successful Enable and leaves the entry untouched on failure.
// Implementations must not overwrite another installation.
type StartupManager interface {
	Enable(context.Context, Info, string) error
	Disable(context.Context, Info) error
}

// TransactionalStartupManager is required to upgrade or uninstall an installation
// that uses startup registration. A nil snapshot represents a missing entry.
// Restore must atomically restore the snapshot, including removing a new entry
// when given nil. Snapshots are opaque to Installer and must be stable on read.
type TransactionalStartupManager interface {
	StartupManager
	Snapshot(context.Context, Info) ([]byte, error)
	Restore(context.Context, Info, []byte) error
}

type Launcher interface{ Launch(string) error }
type Option func(*Installer)

func WithStartupManager(m StartupManager) Option { return func(i *Installer) { i.startup = m } }
func WithLauncher(l Launcher) Option             { return func(i *Installer) { i.launcher = l } }

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,127}$`)

func validateConfig(c Config) error {
	if !validID.MatchString(c.ID) || !safePath(c.ID) {
		return fmt.Errorf("ID must contain lowercase letters, digits, dots or hyphens")
	}
	if c.Name == "" || c.Version == "" || c.Payload == nil {
		return fmt.Errorf("name, version and payload are required")
	}
	if !safePath(c.Executable) {
		return fmt.Errorf("executable must be a safe relative payload path")
	}
	for _, p := range c.Executables {
		if !safePath(p) {
			return fmt.Errorf("invalid executable path %q", p)
		}
	}
	for _, p := range c.PreservePaths {
		if !safePath(p) || reservedPath(p) {
			return fmt.Errorf("invalid preserved path %q", p)
		}
		for _, executable := range append([]string{c.Executable}, c.Executables...) {
			if underPath(executable, p) {
				return fmt.Errorf("preserved path %q contains executable %q", p, executable)
			}
		}
	}
	if c.MaxBytes < 0 {
		return fmt.Errorf("max bytes must not be negative")
	}
	return nil
}

func defaultDirectory(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "Programs", id), nil
	case "darwin":
		return filepath.Join(home, "Applications", id), nil
	default:
		return filepath.Join(home, ".local", "share", id), nil
	}
}
