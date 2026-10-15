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
	Status   string   `json:"status"`
	Progress Progress `json:"progress"`
	Error    string   `json:"error"`
	Result   *Result  `json:"result"`
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
		state.Result = &result
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
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return Result{}, ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state = State{Status: "installing", Progress: Progress{Phase: "preparing"}}
	s.mu.Unlock()
	defer cancel()
	result, err := s.installer.Install(ctx, request, func(p Progress) { s.mu.Lock(); s.state.Progress = p; s.mu.Unlock() })
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
