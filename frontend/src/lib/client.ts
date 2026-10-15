import type { InstallerBindings, InstallerClient, InstallerState } from './types.js';

export function createClient(bindings: InstallerBindings): InstallerClient {
  return {
    info: () => bindings.Info(),
    state: async () => {
      const state = await bindings.State();
      if (!['ready', 'installing', 'complete', 'error', 'cancelled'].includes(state.status)) {
        throw new Error(`Unknown installer status: ${state.status}`);
      }
      return { ...state, status: state.status as InstallerState['status'], result: state.result ?? null };
    },
    browse: () => bindings.Browse(),
    install: (request) => bindings.Install(request),
    cancel: () => bindings.Cancel(),
    launch: () => bindings.Launch()
  };
}

/** Wails can reject promises with strings as well as Error objects. */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
