import type { InstallerClient, InstallerState, InstallResult } from '../src/lib/types.js';

// Browser preview only. It never writes files or registers startup entries.
export function previewClient(): InstallerClient {
  let state: InstallerState = { status: 'ready', progress: { phase: '', file: '', copiedBytes: 0, totalBytes: 24 * 1048576, percent: 0 }, error: '', result: null };
  let cancelled = false;
  return {
    async info() { return { id: 'io.example.lumen', name: 'Lumen', version: '1.0.0', description: 'A quieter workspace for notes, plans, and the ideas you want to keep.', defaultDirectory: '/home/you/.local/share/lumen', allowStartup: true, platform: 'linux' }; },
    async state() { return structuredClone(state); },
    async browse() { return '/home/you/Applications/Lumen'; },
    async install(request) {
      cancelled = false; state.status = 'installing'; state.error = '';
      for (let n = 0; n <= 20; n++) {
        await new Promise(resolve => setTimeout(resolve, 140));
        if (cancelled) { state.status = 'cancelled'; state.error = 'Installation cancelled.'; throw new Error(state.error); }
        state.progress = { phase: n === 20 ? 'finishing' : 'copying', file: n < 10 ? 'lumen' : 'resources/workspace.dat', copiedBytes: n / 20 * 24 * 1048576, totalBytes: 24 * 1048576, percent: n * 5 };
      }
      const result: InstallResult = { directory: request.directory, executable: `${request.directory}/lumen`, startup: request.startup };
      state.status = 'complete'; state.result = result; return result;
    },
    async cancel() { cancelled = true; },
    async launch() { /* Preview intentionally does not launch a process. */ }
  };
}
