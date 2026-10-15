import { createClient, type InstallerBindings } from '@wails-installer/svelte';

// Equivalent to importing generated wailsjs/go/installer/Service functions.
// This lookup lets Vite build before the Wails CLI has generated bindings.
interface WindowRuntime { Quit(): void; WindowMinimise(): void }
declare global {
  interface Window {
    go?: { installer?: { Service?: InstallerBindings } };
    runtime?: WindowRuntime;
  }
}

export function windowControls() {
  const runtime = window.runtime;
  if (!runtime) throw new Error('Window controls require the Wails desktop runtime.');
  return {
    close: () => runtime.Quit(),
    minimize: () => runtime.WindowMinimise()
  };
}

export function backend() {
  const bindings = window.go?.installer?.Service;
  if (!bindings) throw new Error('Open this example with wails dev. For a browser preview, run npm run dev from the repository root.');
  return createClient(bindings);
}
