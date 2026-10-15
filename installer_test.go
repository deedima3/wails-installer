package installer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{ID: "io.example.test", Name: "Test app", Version: "1.0.0", Executable: "bin/app", DefaultDirectory: filepath.Join(t.TempDir(), "application"), AllowStartup: true,
		Payload: fstest.MapFS{"bin/app": {Data: []byte("executable")}, "data/readme.txt": {Data: []byte("hello")}}}
}

func engine(t *testing.T, c Config, options ...Option) *Installer {
	t.Helper()
	i, err := New(c, options...)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

type fakeStartup struct {
	enabled bool
	path    string
	err     error
}

func (s *fakeStartup) Enable(_ context.Context, _ Info, path string) error {
	if s.err != nil {
		return s.err
	}
	s.enabled, s.path = true, path
	return nil
}
func (s *fakeStartup) Disable(context.Context, Info) error { s.enabled = false; return nil }

type fakeLauncher struct{ path string }

func (l *fakeLauncher) Launch(path string) error { l.path = path; return nil }

func assertNoInstallArtifacts(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination still exists: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftover artifacts: %v", entries)
	}
}

func TestInstallCopiesPayloadAndRegistersStartup(t *testing.T) {
	c := testConfig(t)
	startup := &fakeStartup{}
	i := engine(t, c, WithStartupManager(startup))
	var updates []Progress
	result, err := i.Install(context.Background(), Request{c.DefaultDirectory, true}, func(p Progress) { updates = append(updates, p) })
	if err != nil {
		t.Fatal(err)
	}
	if !startup.enabled || startup.path != result.Executable || !result.Startup {
		t.Fatalf("startup/result mismatch: %+v %+v", startup, result)
	}
	for path, expected := range map[string]string{"bin/app": "executable", "data/readme.txt": "hello"} {
		data, err := os.ReadFile(filepath.Join(result.Directory, filepath.FromSlash(path)))
		if err != nil || string(data) != expected {
			t.Fatalf("%s: %q %v", path, data, err)
		}
	}
	stat, err := os.Stat(result.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && stat.Mode().Perm()&0111 == 0 {
		t.Fatal("entrypoint is not executable")
	}
	previous := 0
	for _, p := range updates {
		if p.Percent < previous || p.Percent > 100 {
			t.Fatalf("non-monotonic progress: %+v", updates)
		}
		previous = p.Percent
	}
	last := updates[len(updates)-1]
	if last.Percent != 100 || last.CopiedBytes != 15 || last.Phase != "complete" {
		t.Fatalf("bad final progress: %+v", last)
	}
}

func TestExistingDestinationIsUntouched(t *testing.T) {
	c := testConfig(t)
	i := engine(t, c)
	if err := os.Mkdir(c.DefaultDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(c.DefaultDirectory, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil); err == nil {
		t.Fatal("accepted existing destination")
	}
	data, _ := os.ReadFile(sentinel)
	if string(data) != "keep" {
		t.Fatal("modified existing file")
	}
}

func TestRejectsInvalidPayload(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload fstest.MapFS
		max     int64
	}{
		{"symlink", fstest.MapFS{"app": {Mode: fs.ModeSymlink, Data: []byte("elsewhere")}}, 0},
		{"backslash", fstest.MapFS{"app": {Data: []byte("ok")}, `..\escape`: {Data: []byte("bad")}}, 0},
		{"drive", fstest.MapFS{"app": {Data: []byte("ok")}, "C:escape": {Data: []byte("bad")}}, 0},
		{"reserved", fstest.MapFS{"app": {Data: []byte("ok")}, "NUL.txt": {Data: []byte("bad")}}, 0},
		{"control character", fstest.MapFS{"app": {Data: []byte("ok")}, "bad\tname": {Data: []byte("bad")}}, 0},
		{"case collision", fstest.MapFS{"app": {Data: []byte("ok")}, "APP": {Data: []byte("bad")}}, 0},
		{"too large", fstest.MapFS{"app": {Data: []byte("12345")}}, 4},
		{"missing executable", fstest.MapFS{"other": {Data: []byte("ok")}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig(t)
			c.Executable = "app"
			c.Payload = tc.payload
			c.MaxBytes = tc.max
			i := engine(t, c)
			if _, err := i.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil); err == nil {
				t.Fatal("accepted invalid payload")
			}
			assertNoInstallArtifacts(t, c.DefaultDirectory)
		})
	}
}

func TestCancellationDuringCopyRemovesStage(t *testing.T) {
	c := testConfig(t)
	c.Payload = fstest.MapFS{"bin/app": {Data: []byte(strings.Repeat("x", 512*1024))}}
	i := engine(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := i.Install(ctx, Request{Directory: c.DefaultDirectory}, func(p Progress) {
		if p.CopiedBytes > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	assertNoInstallArtifacts(t, c.DefaultDirectory)
}

func TestHookAndStartupErrorsRollBack(t *testing.T) {
	failure := errors.New("extension failed")
	for _, phase := range []string{"before", "after", "startup"} {
		t.Run(phase, func(t *testing.T) {
			c := testConfig(t)
			hook := func(context.Context, Installation) error { return failure }
			startup := &fakeStartup{}
			switch phase {
			case "before":
				c.Hooks.BeforeCopy = hook
			case "after":
				c.Hooks.AfterCopy = hook
			case "startup":
				startup.err = failure
			}
			i := engine(t, c, WithStartupManager(startup))
			_, err := i.Install(context.Background(), Request{c.DefaultDirectory, true}, nil)
			if !errors.Is(err, failure) {
				t.Fatalf("missing underlying error: %v", err)
			}
			assertNoInstallArtifacts(t, c.DefaultDirectory)
		})
	}
}

func TestHooksCanAddFilesAndStartupCanBeDisabled(t *testing.T) {
	c := testConfig(t)
	c.AllowStartup = false
	c.Hooks.AfterCopy = func(_ context.Context, p Installation) error {
		return os.WriteFile(filepath.Join(p.StageDirectory, "generated.txt"), []byte("generated"), 0644)
	}
	i := engine(t, c, WithStartupManager(nil))
	if _, err := i.Install(context.Background(), Request{c.DefaultDirectory, true}, nil); err == nil {
		t.Fatal("allowed forbidden startup")
	}
	result, err := i.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "generated.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentInstallersDoNotReplaceEachOther(t *testing.T) {
	c := testConfig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	c.Hooks.BeforeCopy = func(context.Context, Installation) error { close(entered); <-release; return nil }
	i := engine(t, c)
	c.Hooks = Hooks{}
	second := engine(t, c)
	done := make(chan error, 1)
	go func() {
		_, err := i.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil)
		done <- err
	}()
	<-entered
	_, sameErr := i.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil)
	_, otherErr := second.Install(context.Background(), Request{Directory: c.DefaultDirectory}, nil)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(sameErr, ErrBusy) || otherErr == nil {
		t.Fatalf("missing lock: %v, %v", sameErr, otherErr)
	}
}

func TestNewAndDirectoryValidation(t *testing.T) {
	for _, mutate := range []func(*Config){func(c *Config) { c.ID = "../bad" }, func(c *Config) { c.Executable = "../app" }, func(c *Config) { c.Name = "" }, func(c *Config) { c.Payload = nil }, func(c *Config) { c.MaxBytes = -1 }, func(c *Config) { c.DefaultDirectory = "relative" }} {
		c := testConfig(t)
		mutate(&c)
		if _, err := New(c); err == nil {
			t.Fatalf("accepted invalid config: %+v", c)
		}
	}
	for _, path := range []string{"", "relative", string(filepath.Separator), filepath.Join(t.TempDir(), "bad\npath")} {
		if _, err := validateDirectory(path); err == nil {
			t.Fatalf("accepted path %q", path)
		}
	}
}

func TestServiceCancellationAndLaunch(t *testing.T) {
	c := testConfig(t)
	entered := make(chan struct{})
	var once sync.Once
	c.Hooks.BeforeCopy = func(ctx context.Context, _ Installation) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	}
	launcher := &fakeLauncher{}
	service := NewService(engine(t, c, WithLauncher(launcher)), nil)
	if err := service.Launch(); err == nil {
		t.Fatal("launched before install")
	}
	if _, err := service.Browse(); err == nil {
		t.Fatal("missing picker must return an error")
	}
	done := make(chan error, 1)
	go func() { _, err := service.Install(Request{Directory: c.DefaultDirectory}); done <- err }()
	<-entered
	if service.State().Status != "installing" {
		t.Fatal("missing running state")
	}
	if _, err := service.Install(Request{Directory: c.DefaultDirectory}); !errors.Is(err, ErrBusy) {
		t.Fatal("allowed concurrent service call")
	}
	service.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if service.State().Status != "cancelled" {
		t.Fatalf("bad cancellation state: %+v", service.State())
	}
	assertNoInstallArtifacts(t, c.DefaultDirectory)
	c.Hooks = Hooks{}
	service = NewService(engine(t, c, WithLauncher(launcher)), func() (string, error) { return c.DefaultDirectory, nil })
	result, err := service.Install(Request{Directory: c.DefaultDirectory})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := service.State()
	snapshot.Result.Executable = "untrusted"
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	if launcher.path != result.Executable {
		t.Fatal("launch trusted a mutable snapshot")
	}
	if !reflect.DeepEqual(service.Info(), service.installer.Info()) {
		t.Fatal("metadata mismatch")
	}
	if path, err := service.Browse(); err != nil || path != c.DefaultDirectory {
		t.Fatalf("picker: %s %v", path, err)
	}
}
