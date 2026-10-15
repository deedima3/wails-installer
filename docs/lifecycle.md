# Upgrades and uninstalling

Keep a separate installer executable outside the installed application directory. Close the application before maintenance. The same setup UI supports fresh installs, upgrades, and uninstalling on Linux, Windows, and macOS.

## Use the setup UI

Enter the existing installation's full path. The component calls `Inspect`, shows the installed and available versions, and offers Upgrade application or Uninstall application. It also offers Manage installation after a successful install or upgrade.

An upgrade uses the payload embedded in the installer you are running. Build and distribute a new installer for each release and target platform. The library does not download releases or compare version text as semantic versions. A caller can impose its own version policy before invoking `Upgrade`.

Uninstall requires a confirmation checkbox. After removal, the UI reports whether the directory was removed and lists personal or changed files that were kept. It does not register the installer in Windows Apps & Features, load/unload macOS launchd jobs, or erase data elsewhere in the user's account.

## Use the Go API

Create an engine with the same `ID` as the existing application and the new payload/version:

```go
engine, err := installer.New(installer.Config{
    ID: "com.example.myapp",
    Name: "My app",
    Version: "2.0.0",
    Payload: payloadFS,
    Executable: "myapp", // myapp.exe on Windows; bundle path on macOS
    PreservePaths: []string{"settings", "data"},
    AllowStartup: true,
})
if err != nil { return err }

installed, err := engine.Inspect(directory)
if err != nil { return err }
if installed == nil { return installer.ErrNotManaged }

result, err := engine.Upgrade(ctx, installer.Request{
    Directory: directory,
    Startup: installed.Startup,
}, nil)
if err != nil { return err }
for _, warning := range result.Warnings { log.Print(warning) }
```

To remove the application later, use an engine configured for the same application ID:

```go
result, err := engine.Uninstall(ctx, directory, nil)
if err != nil { return err }
log.Printf("Removed %d managed files", result.RemovedFiles)
for _, file := range result.PreservedFiles { log.Printf("Kept %s", file) }
for _, warning := range result.Warnings { log.Print(warning) }
```

The engine still requires a non-nil `Payload` and a safe relative `Executable` in its configuration, but uninstall does not read the payload. Reusing the distributed installer satisfies that requirement. `NewService(engine, picker)` exposes `Inspect`, `Upgrade`, and `Uninstall` directly through Wails. Regenerate frontend bindings after updating the Go dependency.

## Ownership receipt

Every new installation writes `.wails-installer.json` in its root. This reserved file records the application ID, version, platform, location, executable, managed file hashes, managed directories, preserved paths, and startup ownership. Hooks cannot provide or replace this reserved file.

`Install` continues to refuse existing directories. `Upgrade` and `Uninstall` require a matching receipt and refuse foreign or unsafe paths. The record is a local ownership record, not a signed defense against a user intentionally changing the receipt. Use trusted payloads and a trusted writable parent directory.

Upgrade checks the hashes of old managed files before replacing them. If a managed file was edited, upgrade fails while leaving the old installation intact. Either back up the change and restore the original file, or explicitly include that mutable path in `PreservePaths`. The executable cannot be preserved. Obsolete unchanged managed files are removed by an upgrade; personal files are copied into the staged replacement.

## Personal data and preserved paths

Files created after installation are personal files and are retained. Modified managed files are also retained by uninstall. A clean installation with no personal data is removed completely, including its owned login entry and empty managed directories.

Use `PreservePaths` for application data that ships with initial defaults but becomes mutable, such as `settings/config.json`. These paths are not claimed as managed files. Existing values win over payload defaults during upgrade, and uninstall keeps them even if unchanged. Preserved paths from previous receipts remain preserved in later versions. Matching is case-insensitive for portability.

Store ordinary user data in the OS's user data/config directories when practical. Uninstall never touches paths outside the selected installation and its owned login registration. It does not provide a delete-all-user-data option.

The built-in copier rejects symlinks and special files during maintenance. Move or handle those through an application-specific maintenance tool rather than letting a generic uninstaller traverse them. Basic permission bits are retained for copied personal data, subject to umask; ACLs, extended attributes, and timestamps are not preserved. Staging requires free space for the new payload plus retained data on upgrade, or retained data alone on uninstall. If staging runs out of space, the original installation remains intact.

## Startup changes

The upgrade request's `Startup` value is the desired setting. The UI initializes it from the installed receipt. The native manager snapshots the old registration, checks its recorded ownership hash, and restores the snapshot if the operation fails. An entry changed outside this installation is refused so another installation's login item is not deleted accidentally.

Custom managers must implement `TransactionalStartupManager` for maintenance that changes an installed login entry. Its snapshots must be stable, with nil representing absence; `Restore` must restore the exact prior state or remove a newly created entry. See [custom startup registration](customization.md#replace-startup-registration).

## Failure and crash recovery

Copying, migration hooks, hashing, and cancellation happen before publication. Upgrade/uninstall then move the original directory to a sibling backup and publish the staged replacement, if any. If publication or startup changes fail, the engine restores the original directory and startup state. The backup is deleted only after commit. Cleanup failures return a warning with its location.

This rollback handles returned errors and cooperative cancellation. A process kill, machine restart, filesystem failure, or power loss can interrupt the directory swap. Directory contents and startup entries are not a single durable filesystem transaction.

After such an interruption, close the application and inspect the original directory, `.<directory-name>-backup-*` siblings, and `.<id>-install-*` or `.<id>-uninstall-*` staging folders. If the original is missing, an intact backup can be renamed back to the original path. Inspect the receipt and login entry before resuming. Do not delete a backup merely because a new directory exists. Once no installer is running and the installation is recovered, remove the stale sibling lock and any stale lock beside its login entry.

The engine refuses to upgrade or uninstall the directory containing its own running executable. Use a separate setup executable, especially on Windows where running application files may be locked. It does not kill the application or schedule file deletion after reboot.

## Existing installations without receipts

Installations made before this feature, including the initial 0.1 library implementation, have no ownership receipt. The library deliberately refuses to adopt or remove an arbitrary existing directory automatically.

For those installations, back up the old directory and user data, close the application, install into a new directory, and migrate personal data into preserved paths or OS user-data directories. Inspect and remove the old installer-created login entry before enabling the new one. Test the new application, then remove the old installation manually. Future upgrades and uninstalling use the new receipt.
