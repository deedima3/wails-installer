<script lang="ts">
  import Installer from '../src/lib/Installer.svelte';
  import WindowBar from '../src/lib/WindowBar.svelte';
  import { previewClient } from './client.js';
  let theme = 'fern';
  let generation = 0;
  let scenario = 'fresh';
  let windowAction = '';
  const themes: Record<string, string> = { fern: '', clay: '--installer-accent:#efb191;--installer-sidebar:#42312d;', midnight: '--installer-accent:#9cc8ed;--installer-sidebar:#1c2b3b;--installer-surface:#f2f5f9;' };
</script>

<svelte:head><meta name="description" content="Interactive browser preview of the reusable Go and Svelte installer library." /></svelte:head>
<div class="page">
  <div class="preview-bar"><a href="https://github.com/deedima3/wails-installer">WAILS / INSTALLER</a><span>Browser preview · no files are installed</span><div><label for="scenario">Scenario</label><select id="scenario" bind:value={scenario}><option value="fresh">New install</option><option value="managed">Existing install</option></select><label for="theme">Theme</label><select id="theme" bind:value={theme}><option value="fern">Fern</option><option value="clay">Clay</option><option value="midnight">Midnight</option></select><button on:click={() => generation++}>Reset ↻</button></div></div>
  <main style={themes[theme]}>
    <WindowBar title="Install Lumen" onClose={() => windowAction = 'Close requested. In Wails this closes the installer.'} onMinimize={() => windowAction = 'Minimize requested. In Wails this minimizes the installer.'} />
    {#key `${generation}-${scenario}`}<Installer client={previewClient(scenario === 'managed')} />{/key}
  </main>
  <p class="window-status" role="status">{windowAction}</p>
  <p class="caption">One Go library. Your application, your identity.</p>
</div>

<style>
  :global(body) { margin:0;background:#e9ece4;color:#28382e;font-family:'Trebuchet MS',sans-serif }
  :global(*) { box-sizing:border-box }
  .page { max-width:1180px;margin:0 auto;padding:36px 40px }
  .preview-bar { display:flex;align-items:center;justify-content:space-between;gap:16px;max-width:1020px;margin:0 auto 34px;font-size:10px;color:#5d695f }
  a { font-size:10px;letter-spacing:1.8px;font-weight:bold;color:#28382e;text-decoration:none }
  .preview-bar > div { display:flex;align-items:center;gap:8px }
  select,button { font:10px 'Trebuchet MS',sans-serif;border:1px solid #cbd1c4;background:transparent;padding:7px 9px;border-radius:5px;color:#28382e }
  button { cursor:pointer }
  main { max-width:1020px;margin:0 auto;box-shadow:0 24px 60px #26382d12;border-radius:16px;overflow:hidden }
  .window-status { text-align:center;font-size:11px;min-height:16px }
  .caption { text-align:center;font-size:10px;color:#6b776c;letter-spacing:.3px;margin-top:24px }
  @media(max-width:700px) { .page { padding:20px 16px }.preview-bar { flex-wrap:wrap;margin-bottom:20px }.preview-bar > span { order:3;width:100% }.preview-bar > div { flex-wrap:wrap } }
</style>
