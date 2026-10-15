package installer

import (
	"context"
	"errors"
	"sync"
)

// DirectoryPicker returns the full, not-yet-existing install directory. Native
// dialogs typically select its parent; append your application directory there.
type DirectoryPicker func() (string, error)

type State struct {
	Status          string           `json:"status"`
	Progress        Progress         `json:"progress"`
	Error           string           `json:"error"`
	Result          *Result          `json:"result"`
	UninstallResult *UninstallResult `json:"uninstallResult"`
}

// Service is a small binding-safe API for Wails v2 Bind or v3 NewService.
// Poll State while Install's promise is pending. No event names or generated
// binding paths are hard-coded into the frontend package.
type Service struct {
	installer *Installer
	picker    DirectoryPicker
	mu        sync.Mutex
	state     State
	cancel    context.CancelFunc
}

func NewService(i *Installer, picker DirectoryPicker) *Service {
	if i == nil {
		panic("installer: nil Installer")
	}
	return &Service{installer: i, picker: picker, state: State{Status: "ready"}}
}

func (s *Service) Info() Info { return s.installer.Info() }

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	if state.Result != nil {
		result := *state.Result
		result.Warnings = append([]string(nil), result.Warnings...)
		state.Result = &result
	}
	if state.UninstallResult != nil {
		result := *state.UninstallResult
		result.Warnings = append([]string(nil), result.Warnings...)
		result.PreservedFiles = append([]string(nil), result.PreservedFiles...)
		state.UninstallResult = &result
	}
	return state
}

func (s *Service) Browse() (string, error) {
	if s.picker == nil {
		return "", errors.New("directory picker is not configured; enter a directory manually")
	}
	return s.picker()
}

func (s *Service) Install(request Request) (Result, error) {
	return s.runInstall(request, false)
}

func (s *Service) Upgrade(request Request) (Result, error) {
	return s.runInstall(request, true)
}

func (s *Service) Inspect(directory string) (*ManagedInstallation, error) {
	return s.installer.Inspect(directory)
}

func (s *Service) runInstall(request Request, upgrade bool) (Result, error) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return Result{}, ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state = State{Status: "installing", Progress: Progress{Phase: "preparing"}}
	if upgrade {
		s.state.Status = "upgrading"
	}
	s.mu.Unlock()
	defer cancel()
	report := func(p Progress) { s.mu.Lock(); s.state.Progress = p; s.mu.Unlock() }
	var result Result
	var err error
	if upgrade {
		result, err = s.installer.Upgrade(ctx, request, report)
	} else {
		result, err = s.installer.Install(ctx, request, report)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
	if err != nil {
		s.state.Status, s.state.Error = "error", err.Error()
		if errors.Is(err, context.Canceled) {
			s.state.Status = "cancelled"
		}
	} else {
		s.state.Status, s.state.Result = "complete", &result
	}
	return result, err
}

func (s *Service) Uninstall(directory string) (UninstallResult, error) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return UninstallResult{}, ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state = State{Status: "uninstalling", Progress: Progress{Phase: "preparing"}}
	s.mu.Unlock()
	defer cancel()
	result, err := s.installer.Uninstall(ctx, directory, func(p Progress) { s.mu.Lock(); s.state.Progress = p; s.mu.Unlock() })
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
	if err != nil {
		s.state.Status, s.state.Error = "error", err.Error()
		if errors.Is(err, context.Canceled) {
			s.state.Status = "cancelled"
		}
	} else {
		s.state.Status, s.state.UninstallResult = "uninstalled", &result
	}
	return result, err
}

func (s *Service) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// Launch uses only the path from the last successful install, never a path
// supplied by the frontend. Launching is an explicit, separate user action.
func (s *Service) Launch() error {
	s.mu.Lock()
	if s.state.Status != "complete" || s.state.Result == nil {
		s.mu.Unlock()
		return errors.New("install the application before launching it")
	}
	executable := s.state.Result.Executable
	s.mu.Unlock()
	return s.installer.launcher.Launch(executable)
}
