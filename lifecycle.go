package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (i *Installer) install(ctx context.Context, request Request, upgrade bool, report func(Progress)) (result Result, err error) {
	if !i.mu.TryLock() {
		return result, ErrBusy
	}
	defer i.mu.Unlock()
	if request.Startup && !i.config.AllowStartup {
		return result, errors.New("launch at login is disabled by this application")
	}
	dir, err := absoluteDirectory(request.Directory)
	if err != nil {
		return result, err
	}
	if !upgrade {
		if _, err := validateDirectory(dir); err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	release, err := reserveDirectory(dir)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, release()) }()
	var old *receipt
	paths := append([]string(nil), i.config.PreservePaths...)
	if upgrade {
		old, err = i.readReceipt(dir)
		if err != nil {
			return result, err
		}
		if err = requireExternalProcess(dir); err != nil {
			return result, err
		}
		paths = append(paths, old.PreservePaths...)
	}
	entries, total, err := i.inventory(ctx)
	if err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), "."+i.config.ID+"-install-*")
	if err != nil {
		return result, err
	}
	defer func() {
		if e := os.RemoveAll(stage); e != nil {
			err = errors.Join(err, fmt.Errorf("clean staging directory %s: %w", stage, e))
		}
	}()
	progress := Progress{Phase: "preparing", TotalBytes: total}
	emit := func() {
		if report != nil {
			report(progress)
		}
	}
	emit()
	retainedFiles, retainedDirectories := map[string]bool{}, map[string]bool{}
	if old != nil {
		progress.Phase = "preserving"
		emit()
		retainedFiles, retainedDirectories, _, err = cloneSurvivors(ctx, dir, stage, old, paths, true)
		if err != nil {
			return result, err
		}
	}
	installation := Installation{StageDirectory: stage, Directory: dir, Executable: filepath.Join(dir, filepath.FromSlash(i.config.Executable))}
	if hook := i.config.Hooks.BeforeCopy; hook != nil {
		if err = hook(ctx, installation); err != nil {
			return result, fmt.Errorf("before copy: %w", err)
		}
	}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		target := filepath.Join(stage, filepath.FromSlash(entry.path))
		if preserved(entry.path, paths) {
			if _, statErr := os.Lstat(target); statErr == nil {
				continue
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return result, statErr
			}
		}
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return result, err
		}
		progress.Phase, progress.File = "copying", entry.path
		err = i.copyFile(ctx, entry, target, func(n int64) {
			progress.CopiedBytes += n
			if total > 0 {
				progress.Percent = int(float64(progress.CopiedBytes) / float64(total) * 95)
			}
			emit()
		})
		if err != nil {
			return result, fmt.Errorf("copy %s, an existing personal file may conflict: %w", entry.path, err)
		}
	}
	if hook := i.config.Hooks.AfterCopy; hook != nil {
		if err = hook(ctx, installation); err != nil {
			return result, fmt.Errorf("after copy: %w", err)
		}
	}
	record, err := i.stageReceipt(ctx, stage, dir, retainedFiles, retainedDirectories, paths)
	if err != nil {
		return result, err
	}
	if err = writeReceipt(stage, record); err != nil {
		return result, err
	}
	transition, err := i.prepareStartup(ctx, old, request.Startup, installation.Executable)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, transition.release()) }()
	progress.Phase, progress.File, progress.Percent = "finishing", "", 97
	emit()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	mode := fs.FileMode(0755)
	if upgrade {
		original, statErr := os.Stat(dir)
		if statErr != nil {
			return result, statErr
		}
		mode = original.Mode().Perm()
	}
	if err = os.Chmod(stage, mode); err != nil {
		return result, err
	}
	backup, err := publishDirectory(dir, stage, upgrade, true)
	if err != nil {
		return result, err
	}
	if err = transition.apply(ctx, record); err == nil {
		err = writeReceipt(dir, record)
	}
	if err != nil {
		return result, errors.Join(err, rollbackDirectory(dir, stage, backup, true), transition.rollback())
	}
	result = Result{Directory: dir, Executable: installation.Executable, Startup: request.Startup}
	result.Warnings = cleanupBackup(backup)
	progress.Phase, progress.Percent = "complete", 100
	emit()
	return result, nil
}

// Uninstall removes unchanged owned files. Personal files, configured preserved
// paths, and modified managed files survive. Run from a separate installer.
func (i *Installer) Uninstall(ctx context.Context, directory string, report func(Progress)) (result UninstallResult, err error) {
	if !i.mu.TryLock() {
		return result, ErrBusy
	}
	defer i.mu.Unlock()
	dir, err := absoluteDirectory(directory)
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	release, err := reserveDirectory(dir)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, release()) }()
	old, err := i.readReceipt(dir)
	if err != nil {
		return result, err
	}
	if err = requireExternalProcess(dir); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), "."+i.config.ID+"-uninstall-*")
	if err != nil {
		return result, err
	}
	defer func() {
		if e := os.RemoveAll(stage); e != nil {
			err = errors.Join(err, e)
		}
	}()
	if report != nil {
		report(Progress{Phase: "preserving"})
	}
	paths := append(append([]string(nil), old.PreservePaths...), i.config.PreservePaths...)
	_, _, removed, err := cloneSurvivors(ctx, dir, stage, old, paths, false)
	if err != nil {
		return result, err
	}
	if hook := i.config.Hooks.BeforeUninstall; hook != nil {
		if err = hook(ctx, Installation{StageDirectory: stage, Directory: dir, Executable: filepath.Join(dir, filepath.FromSlash(old.Executable))}); err != nil {
			return result, fmt.Errorf("before uninstall: %w", err)
		}
	}
	transition, err := i.prepareStartup(ctx, old, false, "")
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, transition.release()) }()
	kept, hasSurvivors, err := listSurvivors(ctx, stage)
	if err != nil {
		return result, err
	}
	if report != nil {
		report(Progress{Phase: "removing", Percent: 97})
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if hasSurvivors {
		original, statErr := os.Stat(dir)
		if statErr != nil {
			return result, statErr
		}
		if err = os.Chmod(stage, original.Mode().Perm()); err != nil {
			return result, err
		}
	}
	backup, err := publishDirectory(dir, stage, true, hasSurvivors)
	if err != nil {
		return result, err
	}
	if err = transition.apply(ctx, nil); err != nil {
		return result, errors.Join(err, rollbackDirectory(dir, stage, backup, hasSurvivors), transition.rollback())
	}
	result = UninstallResult{Directory: dir, RemovedFiles: removed, DirectoryRemoved: !hasSurvivors}
	result.PreservedFiles = kept
	result.Warnings = cleanupBackup(backup)
	if report != nil {
		report(Progress{Phase: "complete", Percent: 100})
	}
	return result, nil
}

func listSurvivors(ctx context.Context, stage string) ([]string, bool, error) {
	var files []string
	any := false
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
		any = true
		relative, err := filepath.Rel(stage, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !safePath(relative) || reservedPath(relative) {
			return fmt.Errorf("reserved or unsafe kept path %q", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("kept path %q is not a regular file", relative)
		}
		files = append(files, relative)
		return nil
	})
	sort.Strings(files)
	return files, any, err
}

func reserveDirectory(dir string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return nil, err
	}
	lock := filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+".wails-installer-lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return nil, fmt.Errorf("reserve installation directory: %w", err)
	}
	return func() error {
		if err := os.Remove(lock); err != nil {
			return fmt.Errorf("remove lock %s: %w", lock, err)
		}
		return nil
	}, nil
}

func requireExternalProcess(dir string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	relative, err := filepath.Rel(dir, executable)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("run upgrade or uninstall from a separate installer outside the installation directory")
	}
	return nil
}

func cloneSurvivors(ctx context.Context, dir, stage string, old *receipt, paths []string, upgrade bool) (files, directories map[string]bool, removed int, err error) {
	files, directories = map[string]bool{}, map[string]bool{}
	ownedDirectories := map[string]bool{}
	for _, name := range old.Directories {
		ownedDirectories[name] = true
	}
	err = filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if name == dir {
			return nil
		}
		relative, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == receiptName {
			return nil
		}
		if !safePath(relative) || reservedPath(relative) {
			return fmt.Errorf("unsafe installed path %q", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(stage, filepath.FromSlash(relative))
		if info.IsDir() {
			if !ownedDirectories[relative] || preserved(relative, paths) {
				directories[relative] = true
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("close the application and remove or relocate non-regular file %q before maintenance", relative)
		}
		if expected, owned := old.Files[relative]; owned && !preserved(relative, paths) {
			hash, err := hashFile(ctx, name)
			if err != nil {
				return err
			}
			if hash == expected {
				removed++
				return nil
			}
			if upgrade {
				return fmt.Errorf("managed file %q was modified; back up changes or configure PreservePaths before upgrading", relative)
			}
		}
		if err := copyExisting(ctx, name, target, info.Mode().Perm()); err != nil {
			return err
		}
		files[relative] = true
		return nil
	})
	if err != nil {
		return files, directories, removed, err
	}
	if upgrade {
		return files, directories, removed, nil
	}
	// Only remove empty directories known to have been created by installation.
	var names []string
	for name := range ownedDirectories {
		if !directories[name] {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(a, b int) bool { return len(names[a]) > len(names[b]) })
	for _, name := range names {
		target := filepath.Join(stage, filepath.FromSlash(name))
		children, e := os.ReadDir(target)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return files, directories, removed, e
		}
		if len(children) == 0 {
			if e := os.Remove(target); e != nil {
				return files, directories, removed, e
			}
		}
	}
	return files, directories, removed, nil
}

func copyExisting(ctx context.Context, source, target string, mode fs.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, contextReader{ctx, in})
	return errors.Join(err, out.Sync(), out.Close())
}

func publishDirectory(dir, stage string, replace, hasReplacement bool) (string, error) {
	backup := ""
	if replace {
		var err error
		backup, err = os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+"-backup-*")
		if err != nil {
			return "", err
		}
		if err = os.Remove(backup); err != nil {
			return "", err
		}
		if err = os.Rename(dir, backup); err != nil {
			return "", fmt.Errorf("close the application before maintenance: %w", err)
		}
	} else if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("destination already exists or cannot be inspected")
	}
	if hasReplacement {
		if err := os.Rename(stage, dir); err != nil {
			if backup != "" {
				return "", errors.Join(fmt.Errorf("publish installation: %w", err), restoreBackup(dir, backup))
			}
			return "", err
		}
	}
	return backup, nil
}

func restoreBackup(dir, backup string) error {
	if err := os.Rename(backup, dir); err != nil {
		return fmt.Errorf("restore previous installation manually from %s: %w", backup, err)
	}
	return nil
}

func rollbackDirectory(dir, stage, backup string, hasReplacement bool) error {
	var err error
	if hasReplacement {
		err = os.Rename(dir, stage)
		if err != nil {
			return fmt.Errorf("rollback could not move %s aside; previous files are in %s: %w", dir, backup, err)
		}
	}
	if backup != "" {
		return restoreBackup(dir, backup)
	}
	return nil
}

func cleanupBackup(backup string) []string {
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return []string{fmt.Sprintf("close the application and remove backup %s: %v", backup, err)}
		}
	}
	return nil
}

type startupTransition struct {
	apply    func(context.Context, *receipt) error
	rollback func() error
	release  func() error
}

func (i *Installer) prepareStartup(ctx context.Context, old *receipt, enabled bool, executable string) (startupTransition, error) {
	release := func() error { return nil }
	if _, native := i.startup.(nativeStartup); native && (enabled || old != nil && old.Startup) {
		name, err := startupPath(i.info.ID)
		if err != nil {
			return startupTransition{}, err
		}
		if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return startupTransition{}, err
		}
		lock := name + ".wails-installer-lock"
		if err = os.Mkdir(lock, 0700); err != nil {
			return startupTransition{}, fmt.Errorf("another installer may be changing this app's startup entry: %w", err)
		}
		release = func() error { return os.Remove(lock) }
	}
	transition, err := i.prepareStartupUnlocked(ctx, old, enabled, executable)
	if err != nil {
		return startupTransition{}, errors.Join(err, release())
	}
	transition.release = release
	return transition, nil
}

func (i *Installer) prepareStartupUnlocked(ctx context.Context, old *receipt, enabled bool, executable string) (startupTransition, error) {
	noop := startupTransition{apply: func(context.Context, *receipt) error { return nil }, rollback: func() error { return nil }}
	wasEnabled := old != nil && old.Startup
	if !wasEnabled && !enabled {
		return noop, nil
	}
	if i.startup == nil {
		return noop, errors.New("startup manager is required to change this installation's login entry")
	}
	manager, transactional := i.startup.(TransactionalStartupManager)
	if !transactional {
		if old != nil {
			return noop, errors.New("upgrade/uninstall with startup requires TransactionalStartupManager")
		}
		created := false
		return startupTransition{apply: func(ctx context.Context, record *receipt) error {
			if err := i.startup.Enable(ctx, i.info, executable); err != nil {
				return err
			}
			created = true
			record.Startup = true
			return nil
		}, rollback: func() error {
			if created {
				return i.startup.Disable(context.Background(), i.info)
			}
			return nil
		}}, nil
	}
	before, err := manager.Snapshot(ctx, i.info)
	if err != nil {
		return noop, err
	}
	if before != nil {
		if !wasEnabled {
			return noop, errors.New("a startup entry already exists for this application ID")
		}
		if old.StartupHash == "" || digest(before) != old.StartupHash {
			return noop, errors.New("startup entry changed outside this installation; inspect it before maintenance")
		}
	}
	changed := false
	return startupTransition{
		apply: func(ctx context.Context, record *receipt) error {
			if wasEnabled && before != nil {
				if err := manager.Disable(ctx, i.info); err != nil {
					return err
				}
				changed = true
			}
			if enabled {
				if err := manager.Enable(ctx, i.info, executable); err != nil {
					return err
				}
				changed = true
				after, err := manager.Snapshot(ctx, i.info)
				if err != nil {
					return err
				}
				if after == nil {
					return errors.New("startup manager did not create a registration")
				}
				record.Startup, record.StartupHash = true, digest(after)
			}
			return nil
		},
		rollback: func() error {
			if changed {
				return manager.Restore(context.Background(), i.info, before)
			}
			return nil
		},
	}, nil
}
