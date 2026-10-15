package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const receiptName = ".wails-installer.json"
const maxReceiptBytes = 8 << 20

var ErrNotManaged = errors.New("directory is not a managed installation of this application")

type receipt struct {
	Schema        int               `json:"schema"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Platform      string            `json:"platform"`
	Directory     string            `json:"directory"`
	Executable    string            `json:"executable"`
	Startup       bool              `json:"startup"`
	StartupHash   string            `json:"startupHash,omitempty"`
	Files         map[string]string `json:"files"`
	Directories   []string          `json:"directories"`
	PreservePaths []string          `json:"preservePaths"`
}

func underPath(name, prefix string) bool {
	name, prefix = strings.ToLower(name), strings.ToLower(prefix)
	return name == prefix || strings.HasPrefix(name, prefix+"/")
}

func preserved(name string, paths []string) bool {
	for _, prefix := range paths {
		if underPath(name, prefix) {
			return true
		}
	}
	return false
}

func reservedPath(name string) bool { return underPath(name, receiptName) }

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func hashFile(ctx context.Context, name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx, file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func absoluteDirectory(dir string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\x00\r\n") {
		return "", errors.New("choose an absolute installation directory")
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Dir(dir) {
		return "", errors.New("cannot manage a filesystem root")
	}
	return dir, nil
}

func (i *Installer) readReceipt(dir string) (*receipt, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrNotManaged
	}
	directoryInfo := info
	file := filepath.Join(dir, receiptName)
	info, err = os.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotManaged
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxReceiptBytes {
		return nil, errors.New("invalid installation receipt")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var record receipt
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("read installation receipt: %w", err)
	}
	sameDirectory := record.Directory == dir || (runtime.GOOS == "windows" && strings.EqualFold(record.Directory, dir))
	if !sameDirectory && filepath.IsAbs(record.Directory) {
		if recordedInfo, statErr := os.Stat(record.Directory); statErr == nil {
			sameDirectory = os.SameFile(directoryInfo, recordedInfo)
		}
	}
	if record.Schema != 1 || record.ID != i.config.ID || record.Platform != runtime.GOOS || !sameDirectory || !safePath(record.Executable) || record.Files == nil {
		return nil, ErrNotManaged
	}
	if _, ok := record.Files[record.Executable]; !ok {
		return nil, errors.New("receipt does not own its executable")
	}
	for name, hash := range record.Files {
		if !safePath(name) || reservedPath(name) || len(hash) != sha256.Size*2 {
			return nil, errors.New("invalid managed file in receipt")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, errors.New("invalid file checksum in receipt")
		}
	}
	for _, name := range append(append([]string(nil), record.Directories...), record.PreservePaths...) {
		if !safePath(name) || reservedPath(name) {
			return nil, errors.New("invalid directory in receipt")
		}
	}
	if preserved(record.Executable, record.PreservePaths) {
		return nil, errors.New("receipt preserves its executable")
	}
	return &record, nil
}

// Inspect returns nil for a missing or unowned directory. Corrupt or unsafe
// receipts return errors. Upgrade and Uninstall require a valid owned receipt.
func (i *Installer) Inspect(directory string) (*ManagedInstallation, error) {
	dir, err := absoluteDirectory(directory)
	if err != nil {
		return nil, err
	}
	record, err := i.readReceipt(dir)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotManaged) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ManagedInstallation{ID: record.ID, Name: record.Name, Version: record.Version, Directory: dir, Executable: filepath.Join(dir, filepath.FromSlash(record.Executable)), Startup: record.Startup}, nil
}

func writeReceipt(directory string, record *receipt) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxReceiptBytes {
		return errors.New("installation receipt exceeds size limit")
	}
	temp, err := os.CreateTemp(directory, ".receipt-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	_, err = temp.Write(data)
	err = errors.Join(err, temp.Sync(), temp.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, receiptName))
}

func (i *Installer) stageReceipt(ctx context.Context, stage, dir string, retainedFiles, retainedDirectories map[string]bool, paths []string) (*receipt, error) {
	record := &receipt{Schema: 1, ID: i.config.ID, Name: i.config.Name, Version: i.config.Version, Platform: runtime.GOOS, Directory: dir, Executable: i.config.Executable, Files: map[string]string{}, PreservePaths: paths}
	seen := map[string]bool{}
	err := filepath.WalkDir(stage, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if name == stage {
			return nil
		}
		relative, err := filepath.Rel(stage, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !safePath(relative) || reservedPath(relative) {
			return fmt.Errorf("reserved or unsafe staged path %q", relative)
		}
		folded := strings.ToLower(relative)
		if seen[folded] {
			return fmt.Errorf("case-colliding staged path %q", relative)
		}
		seen[folded] = true
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !retainedDirectories[relative] && !preserved(relative, paths) {
				record.Directories = append(record.Directories, relative)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged path %q must be a regular file", relative)
		}
		if retainedFiles[relative] || preserved(relative, paths) {
			return nil
		}
		hash, err := hashFile(ctx, name)
		if err != nil {
			return err
		}
		record.Files[relative] = hash
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := record.Files[record.Executable]; !ok {
		return nil, errors.New("staged executable is missing or conflicts with retained data")
	}
	return record, nil
}
