# API reference

Import the Go package as `installer "github.com/deedima3/wails-installer"`.

## Config

| Field | Type | Behavior |
| --- | --- | --- |
| `ID` | `string` | Required portable identifier. Lowercase letters, digits, dots, hyphens, starting with a letter or digit. Maximum 128 characters. Cannot be a Windows reserved filename or end in a dot. Keep it stable across releases. |
| `Name`, `Version` | `string` | Required display name and version text. |
| `Description` | `string` | Optional sidebar copy. |
| `Payload` | `fs.FS` | Required trusted payload, already rooted at the application files. Prefer `fs.Sub` on `embed.FS`. |
| `Executable` | `string` | Required slash-separated relative payload path. Must exist for install/upgrade and gets executable permissions. |
| `Executables` | `[]string` | Additional relative paths that need executable permissions. |
| `PreservePaths` | `[]string` | Relative files or directory trees to keep across upgrades and uninstall. These paths must not contain an executable or the reserved ownership receipt. |
| `DefaultDirectory` | `string` | Absolute initial path. Empty chooses the platform default. |
| `AllowStartup` | `bool` | Makes the login option available. False by default and enforced on install/upgrade. Uninstall still removes an owned login entry. |
| `MaxBytes` | `int64` | Maximum payload size. Zero uses 2 GiB. Excludes retained data and hook-generated files. |
| `Hooks` | `Hooks` | Optional `BeforeCopy`, `AfterCopy`, and `BeforeUninstall` functions. |

`New(config, options...) (*Installer, error)` validates configuration. Payload inspection happens at install/upgrade time. Options are `WithStartupManager` and `WithLauncher`. Required implementations must not be nil.

## Go lifecycle methods

| Method | Result | Behavior |
| --- | --- | --- |
| `Info()` | `Info` | Application metadata, initial path, and platform. |
| `Inspect(directory)` | `*ManagedInstallation, error` | Read the on-disk receipt. Missing/unowned directories return nil. Corrupt receipts return an error. |
| `Install(ctx, request, report)` | `Result, error` | Fresh install into a new directory. Writes an ownership receipt. |
| `Upgrade(ctx, request, report)` | `Result, error` | Replace an owned installation with the configured payload. Preserve personal files, check managed file hashes, and restore the previous version if publication/startup changes fail. |
| `Uninstall(ctx, directory, report)` | `UninstallResult, error` | Remove unchanged owned files and the owned login entry. Keep personal files, modified managed files, and preserved paths. |

`report` is an optional synchronous `func(Progress)`. It must be fast and must not panic. Concurrent lifecycle calls on one engine return `ErrBusy`. Separate instances targeting the same directory coordinate through a sibling lock. The native startup manager also locks its application ID during a transaction.

Cancellation is checked during copying/hashing and before publication. Rollback runs without the cancelled context. Once publication, startup changes, and receipt persistence succeed, the operation is committed. Backup cleanup failures are returned in `Warnings`, with the backup path, rather than hiding a committed operation behind an error.

Maintenance requires the application to be closed and the installer process to live outside the installation directory. There is no automatic process termination. Payload version text is not semver-compared; callers decide which release to supply. See [upgrade and uninstall guide](lifecycle.md).

## Bound Service

`NewService(engine, picker) *Service` creates a Wails binding wrapper. Nil engine panics. Optional `DirectoryPicker` is `func() (string, error)` and should return the full application directory, or an empty string on dismissal.

| Method | Result | Behavior |
| --- | --- | --- |
| `Info()` | `Info` | Metadata. |
| `State()` | `State` | Concurrency-safe operation snapshot. |
| `Inspect(directory)` | `*ManagedInstallation, error` | Discover an existing installation after restarting the installer. |
| `Browse()` | `string, error` | Invoke the supplied picker. Nil picker returns an explanatory error. |
| `Install(Request)` | `Result, error` | Run a fresh install. |
| `Upgrade(Request)` | `Result, error` | Upgrade an owned installation. |
| `Uninstall(directory)` | `UninstallResult, error` | Remove managed files, retaining personal/changed files. |
| `Cancel()` | none | Request cancellation of the active lifecycle operation. |
| `Launch()` | `error` | Launch the last successful install/upgrade. Refuses calls after uninstall or a subsequent failed operation. |

Operation state is in memory. Receipts persist on disk and are discoverable through `Inspect`. Starting a new operation clears previous results. Terminal statuses are `complete`, `uninstalled`, `error`, or `cancelled`.

## JSON and TypeScript contract

The Svelte package exports the component, `createClient`, and these public types:

```ts
interface InstallRequest { directory: string; startup: boolean }
interface InstallResult {
  directory: string;
  executable: string;
  startup: boolean;
  warnings?: string[] | null;
}
interface ManagedInstallation {
  id: string;
  name: string;
  version: string;
  directory: string;
  executable: string;
  startup: boolean;
}
interface UninstallResult {
  directory: string;
  removedFiles: number;
  preservedFiles: string[] | null;
  directoryRemoved: boolean;
  warnings?: string[] | null;
}
interface Progress {
  phase: string; // preparing | preserving | copying | finishing | removing | complete
  file: string;
  copiedBytes: number;
  totalBytes: number;
  percent: number;
}
interface InstallerState {
  status: 'ready' | 'installing' | 'upgrading' | 'uninstalling'
    | 'complete' | 'uninstalled' | 'error' | 'cancelled';
  progress: Progress;
  error: string;
  result: InstallResult | null;
  uninstallResult?: UninstallResult | null;
}
```

`InstallerInfo` contains `id`, `name`, `version`, `description`, `defaultDirectory`, `allowStartup`, and `platform`. `InstallerBindings` contains PascalCase promise methods matching Go. Its `BindingState` accepts the broad string status and optional pointers generated by Wails. `createClient` validates statuses and normalizes missing result pointers to null.

`InstallerClient` exposes lower-case `info`, `state`, `browse`, `install`, `cancel`, and `launch`. Maintenance methods `inspect`, `upgrade`, and `uninstall` are optional for compatibility with older install-only backends. `createClient` enables them only when all three generated methods are present. Regenerate bindings after updating the Go library to make the UI's maintenance controls available.

## Svelte props and behavior

| Prop | Default | Purpose |
| --- | --- | --- |
| `client` | required | An `InstallerClient`. Keep its identity stable while mounted. |
| `eyebrow` | `APPLICATION SETUP` | Small heading text. |
| `logoUrl` | empty | Brand image URL. |
| `className` | empty | Extra root CSS class. |
| `onInstalled` | undefined | Callback after a successful install or upgrade initiated by the component. Receives `InstallResult`. |
| `onUninstalled` | undefined | Callback after a successful uninstall initiated by the component. Receives `UninstallResult`. |

Named `sidebar` and `footer` slots and CSS variables customize branding. The component inspects the selected directory, offers maintenance actions for managed installs, and requires explicit confirmation before uninstalling. It reports kept files and cleanup warnings after removal. It polls active operations and removes timers on unmount. Unmounting does not cancel the backend operation.

## Errors and ownership

Use `errors.Is` for `ErrBusy`, `ErrNotManaged`, and `context.Canceled` where applicable. Wails can reject promises with strings as well as `Error` values.

The `.wails-installer.json` receipt is reserved. It records app identity, platform, version, owned file hashes, owned directories, preserved paths, and startup ownership. It is a local ownership record, not a signed security boundary against the same user modifying its contents. Download authentication and release signatures are separate concerns.

Upgrade refuses modified managed files unless they are explicitly preserved. Uninstall keeps those modified files instead. Both operations refuse unsafe paths, symlinks, special files, foreign receipts, and changed startup registrations that cannot be proven owned. Applications installed before receipts existed need the migration described in [the lifecycle guide](lifecycle.md#existing-installations-without-receipts).
