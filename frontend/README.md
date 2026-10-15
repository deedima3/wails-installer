# @wails-installer/svelte

Svelte 4/5 component for the `github.com/deedima3/wails-installer` Go service. This package provides UI only. File installation, cancellation, and startup registration run in Go through Wails bindings.

```svelte
<script lang="ts">
  import { Installer, createClient } from '@wails-installer/svelte';
  import * as bindings from '../wailsjs/go/installer/Service';
  const client = createClient(bindings);
</script>

<div style="--installer-accent:#efb191;--installer-sidebar:#42312d">
  <Installer {client} />
</div>
```

Build with `npm run build` before installing this package from a local checkout. The package exports its Svelte source, JavaScript client adapter, and TypeScript declarations. Your application needs Svelte-aware bundling.

Props include `client`, `logoUrl`, `eyebrow`, `className`, `onInstalled`, and `onUninstalled`. Named slots `sidebar` and `footer` replace those sections. The backend provides the app name, description, version, initial directory, and startup availability.

When `Inspect`, `Upgrade`, and `Uninstall` are present in the generated bindings, the component detects managed installations and offers upgrade/uninstall actions. Uninstall requires confirmation and reports kept personal files. Older install-only bindings remain supported without those controls. Regenerate bindings after updating the Go library.

See the repository's [integration guide](https://github.com/deedima3/wails-installer/blob/main/docs/integration.md), [customization guide](https://github.com/deedima3/wails-installer/blob/main/docs/customization.md), and [API reference](https://github.com/deedima3/wails-installer/blob/main/docs/api.md). These URLs assume publication at the module's declared repository path.
