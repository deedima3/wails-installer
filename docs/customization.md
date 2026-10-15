# Customization and extensions

## Style the Svelte component

Every component style is scoped. Use inherited CSS variables on a wrapper to change the theme without editing the library. There are no remote fonts, icon requests, or external stylesheets.

```svelte
<script lang="ts">
  import { Installer, createClient } from '@wails-installer/svelte';
  import * as bindings from '../wailsjs/go/installer/Service';
  const client = createClient(bindings);
</script>

<div class="my-installer">
  <Installer {client} logoUrl="/brand.svg" eyebrow="MY APP SETUP">
    <div slot="sidebar">
      <h2>Welcome to My app.</h2>
      <p>Your own product copy or illustration goes here.</p>
    </div>
    <div slot="footer">My company · Version 1.0.0</div>
  </Installer>
</div>

<style>
  .my-installer {
    --installer-accent: #efb191;
    --installer-sidebar: #42312d;
    --installer-sidebar-text: #fff5ef;
    --installer-surface: #fffaf6;
    --installer-border: #e3d8d1;
    --installer-ink: #332b27;
    --installer-muted: #74675f;
    --installer-font: 'Your bundled font', 'Trebuchet MS', sans-serif;
    --installer-radius: 12px;
  }
</style>
```

Keep contrast readable when changing colors, including focus outlines, error messages, checkbox controls, and button text. The layout collapses to one column below 600 px and respects reduced-motion preferences.

| Variable | Default | Purpose |
| --- | --- | --- |
| `--installer-accent` | `#d3ef78` | Primary buttons, logo mark, completion indicator |
| `--installer-button-text` | `#202820` | Text on the primary button |
| `--installer-sidebar` | `#26382d` | Sidebar background |
| `--installer-sidebar-text` | `#eff4e9` | Sidebar text |
| `--installer-surface` | `#f9faf5` | Main background |
| `--installer-input` | `#fff` | Path field background |
| `--installer-ink` | `#202820` | Main text and progress bar |
| `--installer-muted` | `#687064` | Secondary text |
| `--installer-border` | `#dedfd6` | Dividers and field borders |
| `--installer-font` | `'Trebuchet MS', sans-serif` | Component typography |
| `--installer-heading-size` | `46px` | Desktop main heading size |
| `--installer-radius` | `16px` | Outer corner radius |
| `--installer-error-bg` | `#fff0e9` | Error background |
| `--installer-error-text` | `#8b2f19` | Error text and outline |

`logoUrl` is an optional image URL served by your app. `eyebrow` changes the small setup heading. `className` adds a class to the component root. The named `sidebar` slot replaces the illustration and descriptive copy while retaining the application brand and bottom status line. The `footer` slot replaces the footer contents.

The default UI copy is English. For localization, additional setup pages, or a different layout, create your own UI against the same `InstallerClient`. App name, version, and description come from Go `Config`. CSS variables and slots cover branding rather than translating every label.

## Change install paths

Set `Config.DefaultDirectory` to an absolute path. The user can edit it in the component. `DirectoryPicker` should return the full new application directory, even when the native dialog selects its existing parent. An empty string means the user dismissed the picker.

If you need to restrict allowed destinations, bind a wrapper service that validates `Request.Directory` before calling `Service.Install`. Browser-side validation alone is not a policy boundary. Do not bind the unrestricted service alongside a wrapper intended to enforce restrictions.

## Generate configuration files

Use hooks for work that belongs inside the staged application:

```go
config.Hooks.AfterCopy = func(ctx context.Context, install installer.Installation) error {
    if err := ctx.Err(); err != nil { return err }
    // StageDirectory is temporary. Directory is the final install location.
    data := []byte("{\"firstRun\":true}\n")
    return os.WriteFile(filepath.Join(install.StageDirectory, "settings.json"), data, 0600)
}
```

Hooks run on the operation goroutine. `BeforeCopy` and `AfterCopy` run for both install and upgrade. On upgrade, staging already contains retained personal files. `BeforeUninstall` runs with a staging directory containing files that will be kept. Errors abort before publication. External side effects need their own rollback. Do not create symlinks inside staging or modify the final destination from a hook. Use `Config.PreservePaths`, such as `[]string{"settings", "data"}`, for mutable application data. See [data preservation](lifecycle.md#personal-data-and-preserved-paths).

## Replace startup registration

Implement `StartupManager` and supply it to `New`:

```go
type CompanyStartup struct{}

func (CompanyStartup) Enable(ctx context.Context, app installer.Info, executable string) error {
    // Register the current user through your platform integration.
    // If this returns an error, leave any previous registration unchanged.
    return registerUserLogin(ctx, app.ID, executable)
}

func (CompanyStartup) Disable(ctx context.Context, app installer.Info) error {
    return removeUserLogin(ctx, app.ID)
}

engine, err := installer.New(config, installer.WithStartupManager(CompanyStartup{}))
```

`registerUserLogin` and `removeUserLogin` stand for your own implementation. `Enable` and `Disable` must leave registration unchanged on failure. The simple interface supports fresh installs. To manage startup during upgrades and uninstall, also implement `TransactionalStartupManager`:

```go
func (CompanyStartup) Snapshot(ctx context.Context, app installer.Info) ([]byte, error) {
    return readUserLogin(ctx, app.ID) // nil means no entry, otherwise a stable snapshot
}

func (CompanyStartup) Restore(ctx context.Context, app installer.Info, snapshot []byte) error {
    return restoreUserLogin(ctx, app.ID, snapshot) // nil removes a newly created entry
}
```

Those helpers are supplied by your integration. Snapshots must be stable across reads, and restoration must be atomic. Serialize operations that share an application ID in your manager if it can be used concurrently across install directories. The library uses snapshots to validate ownership and restore the previous registration if maintenance fails. The built-in manager implements this interface on all three platforms.

## Replace launching

A custom `Launcher` can use an OS launcher or pass application arguments. It receives only the successful install's executable path.

```go
type MyLauncher struct{}

func (MyLauncher) Launch(executable string) error {
    cmd := exec.Command(executable, "--welcome")
    cmd.Dir = filepath.Dir(executable)
    if err := cmd.Start(); err != nil { return err }
    go func() { _ = cmd.Wait() }()
    return nil
}

engine, err := installer.New(config, installer.WithLauncher(MyLauncher{}))
```

The built-in launcher already opens conventional macOS `.app` bundles through `/usr/bin/open`. A custom launcher is useful for a different bundle layout or additional arguments. Use argument arrays rather than interpolating paths into shell commands. Launching occurs only after the user clicks Open. A successful launch call means the OS accepted the launch request, not that the application finished loading.
