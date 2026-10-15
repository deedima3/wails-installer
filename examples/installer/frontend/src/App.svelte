<script lang="ts">
  import { onMount } from 'svelte';
  import { Installer, type InstallerClient } from '@wails-installer/svelte';
  import { backend } from './backend.js';
  let client: InstallerClient | undefined;
  let error = '';
  onMount(() => { try { client = backend(); } catch (e) { error = String(e); } });
</script>

<main>{#if client}<Installer {client} />{:else if error}<p role="alert">{error}</p>{/if}</main>

<style>
  :global(body) { margin:0;background:#e9ece4;font-family:'Trebuchet MS',sans-serif }
  main { padding:20px }
  p { margin:40px;line-height:1.7 }
  @media(max-width:600px) { main { padding:0 } }
</style>
