import type { InstallerBindings, InstallerClient, InstallerState } from './types.js';

export function createClient(bindings: InstallerBindings): InstallerClient {
  return {
    info: () => bindings.Info(),
    state: async () => {
      const state = await bindings.State();
      if (!['ready', 'installing', 'upgrading', 'uninstalling', 'uninstalled', 'complete', 'error', 'cancelled'].includes(state.status)) {
        throw new Error(`Unknown installer status: ${state.status}`);
      }
      return { ...state, status: state.status as InstallerState['status'], result: state.result ?? null, uninstallResult: state.uninstallResult ?? null };
    },
    browse: () => bindings.Browse(),
    install: (request) => bindings.Install(request),
    cancel: () => bindings.Cancel(),
    launch: () => bindings.Launch(),
    ...(bindings.Inspect && bindings.Upgrade && bindings.Uninstall ? {
      inspect: async (directory: string) => (await bindings.Inspect!(directory)) ?? null,
      upgrade: (request: Parameters<InstallerBindings['Install']>[0]) => bindings.Upgrade!(request),
      uninstall: (directory: string) => bindings.Uninstall!(directory)
    } : {})
  };
}

/** Wails can reject promises with strings as well as Error objects. */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
