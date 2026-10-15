package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"testing/fstest"
)

type transactionalStartup struct {
	data        []byte
	failEnable  bool
	failDisable bool
}

func (s *transactionalStartup) Enable(_ context.Context, _ Info, executable string) error {
	if s.failEnable {
		return errors.New("startup enable failed")
	}
	if s.data != nil {
		return errors.New("startup conflict")
	}
	s.data = []byte(executable)
	return nil
}
func (s *transactionalStartup) Disable(context.Context, Info) error {
	if s.failDisable {
		return errors.New("startup disable failed")
	}
	s.data = nil
	return nil
}
func (s *transactionalStartup) Snapshot(context.Context, Info) ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}
func (s *transactionalStartup) Restore(_ context.Context, _ Info, data []byte) error {
	s.data = append([]byte(nil), data...)
	return nil
}

func mustInstall(t *testing.T, c Config, options ...Option) *Installer {
	t.Helper()
	i := engine(t, c, options...)
	if _, err := i.Install(context.Background(), Request{Directory: c.DefaultDirectory, Startup: c.AllowStartup}, nil); err != nil {
		t.Fatal(err)
	}
	return i
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func writeFile(t *testing.T, root, name, data string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeReplacesOwnedFilesPreservesDataAndRemovesObsoleteFiles(t *testing.T) {
	c := testConfig(t)
	c.AllowStartup = false
	c.PreservePaths = []string{"settings"}
	c.Payload.(fstest.MapFS)["settings/config.json"] = &fstest.MapFile{Data: []byte("defaults")}
	i := mustInstall(t, c)
	writeFile(t, c.DefaultDirectory, "notes/private.txt", "personal")
	writeFile(t, c.DefaultDirectory, "settings/config.json", "my settings")
	c.Version = "2.0.0"
	c.Payload = fstest.MapFS{"bin/app": {Data: []byte("new executable")}, "resources/new.txt": {Data: []byte("new")}, "settings/config.json": {Data: []byte("new defaults")}}
	v2 := engine(t, c)
	result, err := v2.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, result.Directory, "bin/app") != "new executable" || readFile(t, result.Directory, "settings/config.json") != "my settings" || readFile(t, result.Directory, "notes/private.txt") != "personal" {
		t.Fatal("upgrade lost application or personal files")
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "data", "readme.txt")); !os.IsNotExist(err) {
		t.Fatal("obsolete managed file remains")
	}
	installed, err := i.Inspect(c.DefaultDirectory)
	if err != nil || installed.Version != "2.0.0" {
		t.Fatalf("inspect after restart: %+v %v", installed, err)
	}
	uninstall, err := v2.Uninstall(context.Background(), c.DefaultDirectory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uninstall.DirectoryRemoved || !reflect.DeepEqual(uninstall.PreservedFiles, []string{"notes/private.txt", "settings/config.json"}) {
		t.Fatalf("bad uninstall result: %+v", uninstall)
	}
	if _, err := os.Stat(result.Executable); !os.IsNotExist(err) {
		t.Fatal("executable remains")
	}
	if readFile(t, result.Directory, "settings/config.json") != "my settings" {
		t.Fatal("settings removed")
	}
	if installed, err := v2.Inspect(c.DefaultDirectory); err != nil || installed != nil {
		t.Fatalf("uninstalled folder still managed: %+v %v", installed, err)
	}
}

func TestUpgradeErrorsAndCancellationKeepOldInstallation(t *testing.T) {
	for _, mode := range []string{"hook", "cancel", "modified", "foreign file", "startup"} {
		t.Run(mode, func(t *testing.T) {
			c := testConfig(t)
			startup := &transactionalStartup{}
			mustInstall(t, c, WithStartupManager(startup))
			before := readFile(t, c.DefaultDirectory, receiptName)
			startupBefore := string(startup.data)
			c.Version = "2"
			c.Payload = fstest.MapFS{"bin/app": {Data: []byte("new")}, "new.txt": {Data: []byte("new")}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "hook":
				c.Hooks.AfterCopy = func(context.Context, Installation) error { return errors.New("migration failed") }
			case "cancel":
				c.Hooks.AfterCopy = func(context.Context, Installation) error { cancel(); return nil }
			case "modified":
				writeFile(t, c.DefaultDirectory, "data/readme.txt", "edited")
			case "foreign file":
				writeFile(t, c.DefaultDirectory, "new.txt", "personal")
			case "startup":
				startup.failEnable = true
			}
			_, err := engine(t, c, WithStartupManager(startup)).Upgrade(ctx, Request{Directory: c.DefaultDirectory, Startup: true}, nil)
			if err == nil {
				t.Fatal("upgrade should fail")
			}
			if readFile(t, c.DefaultDirectory, "bin/app") != "executable" || readFile(t, c.DefaultDirectory, receiptName) != before || string(startup.data) != startupBefore {
				t.Fatalf("old version was not restored: %v", err)
			}
			entries, err := os.ReadDir(filepath.Dir(c.DefaultDirectory))
			if err != nil || len(entries) != 1 {
				t.Fatalf("backup or staging artifacts: %v %v", entries, err)
			}
		})
	}
}

func TestUninstallPreservesModifiedFilesAndRemovesStartup(t *testing.T) {
	c := testConfig(t)
	startup := &transactionalStartup{}
	i := mustInstall(t, c, WithStartupManager(startup))
	writeFile(t, c.DefaultDirectory, "data/readme.txt", "edited")
	writeFile(t, c.DefaultDirectory, "notes.txt", "personal")
	result, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if startup.data != nil || result.RemovedFiles != 1 || !reflect.DeepEqual(result.PreservedFiles, []string{"data/readme.txt", "notes.txt"}) {
		t.Fatalf("uninstall: %+v startup %s", result, startup.data)
	}
	if readFile(t, c.DefaultDirectory, "data/readme.txt") != "edited" {
		t.Fatal("removed modified file")
	}
}

func TestCleanUninstallRemovesDirectory(t *testing.T) {
	c := testConfig(t)
	c.AllowStartup = false
	i := mustInstall(t, c)
	result, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DirectoryRemoved || result.RemovedFiles != 2 {
		t.Fatalf("uninstall result: %+v", result)
	}
	assertNoInstallArtifacts(t, c.DefaultDirectory)
}

func TestUninstallStartupFailureRollsBackFiles(t *testing.T) {
	c := testConfig(t)
	startup := &transactionalStartup{}
	i := mustInstall(t, c, WithStartupManager(startup))
	startup.failDisable = true
	before := readFile(t, c.DefaultDirectory, receiptName)
	if _, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil); err == nil {
		t.Fatal("expected startup error")
	}
	if readFile(t, c.DefaultDirectory, receiptName) != before || readFile(t, c.DefaultDirectory, "bin/app") != "executable" || startup.data == nil {
		t.Fatal("uninstall did not roll back")
	}
}

func TestMaintenanceRefusesUnownedTamperedAndSymlinkPaths(t *testing.T) {
	for _, mode := range []string{"unowned", "other app", "traversal", "changed startup", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			c := testConfig(t)
			startup := &transactionalStartup{}
			i := mustInstall(t, c, WithStartupManager(startup))
			switch mode {
			case "unowned":
				if err := os.Remove(filepath.Join(c.DefaultDirectory, receiptName)); err != nil {
					t.Fatal(err)
				}
			case "other app":
				c.ID = "other.app"
				i = engine(t, c, WithStartupManager(startup))
			case "traversal":
				record, err := i.readReceipt(c.DefaultDirectory)
				if err != nil {
					t.Fatal(err)
				}
				record.Files["../outside"] = digest([]byte("x"))
				if err := writeReceipt(c.DefaultDirectory, record); err != nil {
					t.Fatal(err)
				}
			case "changed startup":
				startup.data = []byte("another installation")
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(c.DefaultDirectory, "link")); err != nil {
					t.Skip("symlinks unavailable:", err)
				}
			}
			if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory, Startup: true}, nil); err == nil {
				t.Fatal("unsafe upgrade accepted")
			}
			if _, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil); err == nil {
				t.Fatal("unsafe uninstall accepted")
			}
			if readFile(t, c.DefaultDirectory, "bin/app") != "executable" {
				t.Fatal("unsafe operation modified files")
			}
		})
	}
}

func TestPreservePathsRejectsExecutableAndReservedReceipt(t *testing.T) {
	for _, name := range []string{"bin", "../data", receiptName} {
		c := testConfig(t)
		c.PreservePaths = []string{name}
		if _, err := New(c); err == nil {
			t.Fatalf("allowed preserved path %q", name)
		}
	}
	c := testConfig(t)
	c.Payload.(fstest.MapFS)[receiptName] = &fstest.MapFile{Data: []byte("fake")}
	if _, err := engine(t, c).Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil); err == nil {
		t.Fatal("payload can overwrite receipt")
	}
}

func TestNativeLifecycleUpdatesAndRemovesStartup(t *testing.T) {
	isolateUserDirectories(t)
	c := testConfig(t)
	i := mustInstall(t, c)
	c.Version = "2"
	c.Executable = "bin/app-v2"
	c.Payload = fstest.MapFS{"bin/app-v2": {Data: []byte("new")}}
	i = engine(t, c)
	if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory, Startup: true}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := (nativeStartup{}).Snapshot(context.Background(), i.Info())
	if err != nil || data == nil {
		t.Fatalf("startup missing: %v", err)
	}
	if _, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil); err != nil {
		t.Fatal(err)
	}
	data, err = (nativeStartup{}).Snapshot(context.Background(), i.Info())
	if err != nil || data != nil {
		t.Fatalf("startup remains: %v", err)
	}
	assertNoInstallArtifacts(t, c.DefaultDirectory)
}

func TestInspectAcceptsParentDirectoryAliases(t *testing.T) {
	c := testConfig(t)
	c.AllowStartup = false
	i := mustInstall(t, c)
	alias := filepath.Join(t.TempDir(), "parent-alias")
	if err := os.Symlink(filepath.Dir(c.DefaultDirectory), alias); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	managed, err := i.Inspect(filepath.Join(alias, filepath.Base(c.DefaultDirectory)))
	if err != nil || managed == nil {
		t.Fatalf("same directory through a parent alias was rejected: %+v %v", managed, err)
	}
}

func TestStartupCanBeChangedDuringUpgrade(t *testing.T) {
	c := testConfig(t)
	startup := &transactionalStartup{}
	i := mustInstall(t, c, WithStartupManager(startup))
	if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory, Startup: false}, nil); err != nil {
		t.Fatal(err)
	}
	if startup.data != nil {
		t.Fatal("startup was not disabled")
	}
	if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory, Startup: true}, nil); err != nil {
		t.Fatal(err)
	}
	managed, err := i.Inspect(c.DefaultDirectory)
	if err != nil || !managed.Startup || startup.data == nil {
		t.Fatalf("startup was not enabled: %+v %v", managed, err)
	}
}

func TestServiceMaintenanceCancellationAndSerialization(t *testing.T) {
	c := testConfig(t)
	c.AllowStartup = false
	mustInstall(t, c)
	entered := make(chan struct{})
	c.Hooks.BeforeUninstall = func(ctx context.Context, _ Installation) error { close(entered); <-ctx.Done(); return ctx.Err() }
	i := engine(t, c)
	service := NewService(i, nil)
	done := make(chan error, 1)
	go func() { _, err := service.Uninstall(c.DefaultDirectory); done <- err }()
	<-entered
	if service.State().Status != "uninstalling" {
		t.Fatal("missing uninstall status")
	}
	if _, err := service.Upgrade(Request{Directory: c.DefaultDirectory}); !errors.Is(err, ErrBusy) {
		t.Fatalf("service allowed concurrent upgrade: %v", err)
	}
	if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory}, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("engine allowed concurrent upgrade: %v", err)
	}
	service.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if readFile(t, c.DefaultDirectory, "bin/app") != "executable" || service.State().Status != "cancelled" {
		t.Fatal("cancelled uninstall changed files")
	}
	c.Hooks = Hooks{}
	service = NewService(engine(t, c), nil)
	if _, err := service.Uninstall(c.DefaultDirectory); err != nil {
		t.Fatal(err)
	}
	if service.State().Status != "uninstalled" || service.State().Result != nil {
		t.Fatal("bad terminal uninstall state")
	}
	if err := service.Launch(); err == nil {
		t.Fatal("launched after uninstall")
	}
}

func TestMaintenancePreservesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	c := testConfig(t)
	c.AllowStartup = false
	i := mustInstall(t, c)
	if err := os.Chmod(c.DefaultDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, c.DefaultDirectory, "personal/private.txt", "private")
	if _, err := i.Upgrade(context.Background(), Request{Directory: c.DefaultDirectory}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Uninstall(context.Background(), c.DefaultDirectory, nil); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{c.DefaultDirectory: 0700, filepath.Join(c.DefaultDirectory, "personal"): 0700, filepath.Join(c.DefaultDirectory, "personal", "private.txt"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("permissions %s: %v %v, want %o", path, info, err, want)
		}
	}
}
