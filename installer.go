package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var ErrBusy = errors.New("an installation is already running")

type Progress struct {
	Phase       string `json:"phase"`
	File        string `json:"file"`
	CopiedBytes int64  `json:"copiedBytes"`
	TotalBytes  int64  `json:"totalBytes"`
	Percent     int    `json:"percent"`
}

type Installer struct {
	config   Config
	info     Info
	startup  StartupManager
	launcher Launcher
	mu       sync.Mutex
}

func New(c Config, options ...Option) (*Installer, error) {
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	if c.DefaultDirectory == "" {
		var err error
		c.DefaultDirectory, err = defaultDirectory(c.ID)
		if err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(c.DefaultDirectory) {
		return nil, errors.New("default directory must be absolute")
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = 2 << 30
	}
	c.Executables = append([]string(nil), c.Executables...)
	c.PreservePaths = append([]string(nil), c.PreservePaths...)
	i := &Installer{config: c, info: Info{c.ID, c.Name, c.Version, c.Description, c.DefaultDirectory, c.AllowStartup, runtime.GOOS}, startup: nativeStartup{}, launcher: processLauncher{}}
	for _, option := range options {
		option(i)
	}
	if i.launcher == nil || (c.AllowStartup && i.startup == nil) {
		return nil, errors.New("launcher and enabled startup manager must not be nil")
	}
	return i, nil
}

func (i *Installer) Info() Info { return i.info }

// Install creates a fresh installation and records its owned files.
func (i *Installer) Install(ctx context.Context, request Request, report func(Progress)) (Result, error) {
	return i.install(ctx, request, false, report)
}

// Upgrade replaces an owned installation, preserving personal files and rolling
// back the old application and startup registration if publication fails.
func (i *Installer) Upgrade(ctx context.Context, request Request, report func(Progress)) (Result, error) {
	return i.install(ctx, request, true, report)
}

type payloadFile struct {
	path string
	size int64
	mode fs.FileMode
}

func safePath(p string) bool {
	if p == "." || !fs.ValidPath(p) || strings.ContainsAny(p, "\\:") || strings.IndexFunc(p, func(r rune) bool { return r < 0x20 }) >= 0 {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		// Enforce portable Windows filenames even when building on Linux.
		if strings.ContainsAny(part, `<>"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return true
}

func validateDirectory(dir string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\x00\r\n") {
		return "", errors.New("choose an absolute installation directory")
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Dir(dir) {
		return "", errors.New("cannot install into a filesystem root")
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("destination must not already exist: %s", dir)
	}
	return dir, nil
}

func (i *Installer) inventory(ctx context.Context) ([]payloadFile, int64, error) {
	var entries []payloadFile
	var total int64
	found := false
	seen := map[string]bool{}
	err := fs.WalkDir(i.config.Payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if !safePath(p) || reservedPath(p) {
			return fmt.Errorf("unsafe or reserved payload path %q", p)
		}
		folded := strings.ToLower(p)
		if seen[folded] {
			return fmt.Errorf("case-colliding payload path %q", p)
		}
		seen[folded] = true
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("payload must contain regular files, found %s", p)
		}
		if info.Size() < 0 || info.Size() > i.config.MaxBytes-total {
			return errors.New("payload exceeds max bytes")
		}
		total += info.Size()
		entries = append(entries, payloadFile{p, info.Size(), info.Mode()})
		if p == i.config.Executable {
			found = true
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inspect payload: %w", err)
	}
	if !found {
		return nil, 0, errors.New("executable is missing from payload")
	}
	return entries, total, nil
}

func (i *Installer) copyFile(ctx context.Context, entry payloadFile, target string, report func(int64)) error {
	source, err := i.config.Payload.Open(entry.path)
	if err != nil {
		return err
	}
	defer source.Close()
	mode := fs.FileMode(0644)
	if entry.mode.Perm()&0111 != 0 || entry.path == i.config.Executable {
		mode = 0755
	}
	for _, p := range i.config.Executables {
		if p == entry.path {
			mode = 0755
		}
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	var copied int64
	buffer := make([]byte, 128*1024)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			if int64(n) > entry.size-copied {
				err = errors.New("payload changed size while copying")
				break
			}
			var written int
			written, err = out.Write(buffer[:n])
			copied += int64(written)
			report(int64(written))
			if err != nil {
				break
			}
			if written != n {
				err = io.ErrShortWrite
				break
			}
		}
		if readErr == io.EOF {
			if copied != entry.size {
				err = io.ErrUnexpectedEOF
			}
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
	}
	if err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}
