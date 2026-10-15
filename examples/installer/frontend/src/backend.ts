import { createClient, type InstallerBindings } from '@wails-installer/svelte';

// Equivalent to importing generated wailsjs/go/installer/Service functions.
// This lookup lets Vite build before the Wails CLI has generated bindings.
declare global { interface Window { go?: { installer?: { Service?: InstallerBindings } } } }

export function backend() {
  const bindings = window.go?.installer?.Service;
  if (!bindings) throw new Error('Open this example with wails dev. For a browser preview, run npm run dev from the repository root.');
  return createClient(bindings);
}
