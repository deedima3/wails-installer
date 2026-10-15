<script lang="ts">
  import { onMount } from 'svelte';
  import { Installer, WindowBar, type InstallerClient } from '@wails-installer/svelte';
  import { backend, windowControls } from './backend.js';
  let client: InstallerClient | undefined;
  let controls: ReturnType<typeof windowControls> | undefined;
  let error = '';
  onMount(() => { try { controls = windowControls(); client = backend(); } catch (e) { error = String(e); } });
</script>

<div class="app">
  {#if controls}<WindowBar title="Install Lumen" onClose={controls.close} onMinimize={controls.minimize} />{/if}
  <main>{#if client}<Installer {client} />{:else if error}<p role="alert">{error}</p>{/if}</main>
</div>

<style>
  :global(body) { margin:0;background:#e9ece4;font-family:'Trebuchet MS',sans-serif }
  .app { display:flex;flex-direction:column;height:100vh;height:100dvh }
  main { flex:1;min-height:0;overflow:auto;padding:20px }
  p { margin:40px;line-height:1.7 }
  @media(max-width:600px) { main { padding:0 } }
</style>
