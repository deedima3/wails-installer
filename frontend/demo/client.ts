import type { InstallerClient, InstallerState, InstallResult, InstallRequest, ManagedInstallation } from '../src/lib/types.js';

// Browser preview only. It never writes files or registers startup entries.
export function previewClient(existing = false): InstallerClient {
  let state: InstallerState = { status: 'ready', progress: { phase: '', file: '', copiedBytes: 0, totalBytes: 24 * 1048576, percent: 0 }, error: '', result: null };
  let cancelled = false;
  let installed: ManagedInstallation | null = existing ? { id: 'io.example.lumen', name: 'Lumen', version: '1.0.0', directory: '/home/you/.local/share/lumen', executable: '/home/you/.local/share/lumen/lumen', startup: true } : null;
  async function copy(request: InstallRequest, upgrade: boolean): Promise<InstallResult> {
    cancelled = false; state.status = upgrade ? 'upgrading' : 'installing'; state.error = ''; state.result = null; state.uninstallResult = null;
    for (let n = 0; n <= 20; n++) {
      await new Promise(resolve => setTimeout(resolve, 140));
      if (cancelled) { state.status = 'cancelled'; state.error = 'Installation cancelled.'; throw new Error(state.error); }
      state.progress = { phase: n === 20 ? 'finishing' : 'copying', file: n < 10 ? 'lumen' : 'resources/workspace.dat', copiedBytes: n / 20 * 24 * 1048576, totalBytes: 24 * 1048576, percent: n * 5 };
    }
    const result: InstallResult = { directory: request.directory, executable: `${request.directory}/lumen`, startup: request.startup };
    installed = { ...result, id: 'io.example.lumen', name: 'Lumen', version: '2.0.0' };
    state.status = 'complete'; state.result = result; return result;
  }
  return {
    async info() { return { id: 'io.example.lumen', name: 'Lumen', version: '2.0.0', description: 'A quieter workspace for notes, plans, and the ideas you want to keep.', defaultDirectory: '/home/you/.local/share/lumen', allowStartup: true, platform: 'linux' }; },
    async state() { return structuredClone(state); },
    async browse() { return '/home/you/Applications/Lumen'; },
    async inspect(directory) { return installed?.directory === directory ? structuredClone(installed) : null; },
    async install(request) { if (installed?.directory === request.directory) throw new Error('Installation already exists.'); return copy(request, false); },
    async upgrade(request) { if (installed?.directory !== request.directory) throw new Error('No managed installation found.'); return copy(request, true); },
    async uninstall(directory) {
      if (installed?.directory !== directory) throw new Error('No managed installation found.');
      cancelled = false; state.status = 'uninstalling'; state.error = ''; state.result = null;
      for (let n = 0; n <= 20; n++) {
        await new Promise(resolve => setTimeout(resolve, 80));
        if (cancelled) { state.status = 'cancelled'; state.error = 'Installation cancelled.'; throw new Error(state.error); }
        state.progress = { phase: n < 15 ? 'preserving' : 'removing', file: '', copiedBytes: 0, totalBytes: 0, percent: n * 5 };
      }
      installed = null;
      const result = { directory, removedFiles: 2, preservedFiles: existing ? ['notes/private.txt'] : [], directoryRemoved: !existing };
      state.status = 'uninstalled'; state.uninstallResult = result; return result;
    },
    async cancel() { cancelled = true; },
    async launch() { /* Preview intentionally does not launch a process. */ }
  };
}
