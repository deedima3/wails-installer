<script lang="ts">
  import { onMount } from 'svelte';
  import { errorMessage } from './client.js';
  import type { InstallerClient, InstallerInfo, InstallerState, InstallResult } from './types.js';

  export let client: InstallerClient;
  export let eyebrow = 'APPLICATION SETUP';
  export let logoUrl = '';
  export let className = '';
  export let onInstalled: ((result: InstallResult) => void) | undefined = undefined;

  let info: InstallerInfo | null = null;
  let state: InstallerState | null = null;
  let directory = '';
  let startup = false;
  let error = '';
  let loading = true;
  let browsing = false;
  let launching = false;
  let launched = false;
  let cancelRequested = false;
  let disposed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let revision = 0;
  let pendingInstall = false;
  $: busy = pendingInstall || state?.status === 'installing';
  $: complete = state?.status === 'complete';
  $: percent = Math.min(100, Math.max(0, state?.progress.percent ?? 0));
  $: step = complete ? 3 : busy ? 2 : 1;

  onMount(() => {
    void load();
    return () => { disposed = true; revision++; clearTimeout(timer); };
  });

  async function load() {
    loading = true; error = '';
    try {
      const [metadata, current] = await Promise.all([client.info(), client.state()]);
      if (disposed) return;
      info = metadata; state = current;
      directory = current.result?.directory ?? metadata.defaultDirectory;
      startup = current.result?.startup ?? false;
      error = current.error;
      if (current.status === 'installing') schedulePoll();
    } catch (e) { error = errorMessage(e); }
    finally { loading = false; }
  }

  function schedulePoll() {
    clearTimeout(timer);
    const currentRevision = revision;
    timer = setTimeout(async () => {
      try {
        const next = await client.state();
        if (disposed || currentRevision !== revision) return;
        // A delayed binding call can leave the backend briefly at ready while
        // its install promise is already pending. Keep the UI locked.
        if (pendingInstall && next.status === 'ready') { schedulePoll(); return; }
        state = next;
        if (next.error) error = next.error;
        if (cancelRequested && next.status === 'installing') await client.cancel();
        if (next.status === 'installing') schedulePoll();
      } catch (e) {
        if (disposed || currentRevision !== revision) return;
        error = `Cannot read installation progress: ${errorMessage(e)}`;
        schedulePoll();
      }
    }, 200);
  }

  async function browse() {
    browsing = true; error = '';
    try { const path = await client.browse(); if (path) directory = path; }
    catch (e) { error = errorMessage(e); }
    finally { browsing = false; }
  }

  async function install() {
    if (!info || busy || browsing) return;
    error = ''; cancelRequested = false; pendingInstall = true;
    state = { status: 'installing', progress: { phase: 'preparing', file: '', copiedBytes: 0, totalBytes: 0, percent: 0 }, error: '', result: null };
    revision++; schedulePoll();
    let result: InstallResult | undefined;
    try {
      result = await client.install({ directory: directory.trim(), startup: info.allowStartup && startup });
      state = { status: 'complete', progress: { phase: 'complete', file: '', copiedBytes: state.progress.totalBytes, totalBytes: state.progress.totalBytes, percent: 100 }, error: '', result };
    } catch (e) {
      error = errorMessage(e);
      state = { ...state, status: 'error', error };
      try { const final = await client.state(); if (final.status !== 'installing') state = final; } catch { /* Keep the actionable install error. */ }
    } finally { pendingInstall = false; revision++; clearTimeout(timer); }
    if (result) onInstalled?.(result);
  }

  async function cancel() {
    cancelRequested = true;
    try { await client.cancel(); }
    catch (e) { error = errorMessage(e); cancelRequested = false; }
  }

  async function launch() {
    launching = true; error = '';
    try { await client.launch(); launched = true; }
    catch (e) { error = errorMessage(e); }
    finally { launching = false; }
  }

  function bytes(value: number) {
    return value >= 1048576 ? `${(value / 1048576).toFixed(1)} MB` : `${Math.ceil(value / 1024)} KB`;
  }
</script>

<section class="installer {className}" aria-label="Application installer">
  <aside class="sidebar">
    <div class="brand">
      {#if logoUrl}<img src={logoUrl} alt="" class="logo" />{:else}<div class="mark" aria-hidden="true">↗</div>{/if}
      <span>{info?.name ?? 'Installer'}<small>DESKTOP APPLICATION</small></span>
    </div>
    <slot name="sidebar">
      <div class="illustration" aria-hidden="true"><div class="orbit"></div><div class="package"><span>↓</span></div><span class="coordinate">LOCAL / YOURS</span></div>
      <h2>A place for<br />your next idea.</h2>
      <p class="sidebar-description">{info?.description || 'A few details, then you’re ready to begin.'}</p>
    </slot>
    <div class="sidebar-bottom"><span class="status-dot"></span>{complete ? 'Installed on your computer' : 'For your computer'}<span class="version">{info ? `v${info.version}` : ''}</span></div>
  </aside>

  <div class="main">
    <header><span class="eyebrow">{eyebrow}</span><span class="platform">{info?.platform ?? 'Desktop'}</span></header>
    <nav aria-label="Installation steps"><ol>{#each ['Configure', 'Install', 'Ready'] as label, index}<li class:active={step === index + 1} class:done={step > index + 1} aria-current={step === index + 1 ? 'step' : undefined}><span>{step > index + 1 ? '✓' : `0${index + 1}`}</span>{label}</li>{/each}</ol></nav>

    <div class="content">
      {#if loading}
        <div class="loading" role="status">Getting things ready…</div>
      {:else if !info}
        <h1>Setup couldn’t load.</h1><p class="intro">The installer could not connect to the application.</p>
        <button class="primary" on:click={load}>Try again <span aria-hidden="true">↗</span></button>
      {:else if complete}
        <div class="success-icon" aria-hidden="true">✓</div>
        <span class="section-number">ALL SET</span><h1>Make yourself<br />at home.</h1>
        <p class="intro">{info.name} is installed and ready to open.</p>
        <div class="receipt"><span>INSTALLED IN</span><code>{state?.result?.directory}</code><p>{state?.result?.startup ? 'Launch at login is enabled.' : 'Launch at login is off.'}</p></div>
        <button class="primary" on:click={launch} disabled={launching}>{launching ? 'Opening…' : launched ? `Open ${info.name} again` : `Open ${info.name}`}<span aria-hidden="true">↗</span></button>
        {#if launched}<p class="launch-note" role="status">Application launched. You can close this installer.</p>{/if}
      {:else if busy}
        <span class="section-number">INSTALLATION IN PROGRESS</span><h1>Moving in.</h1><p class="intro">Setting up {info.name} on your computer.</p>
        <div class="progress-top"><span>{state?.progress.phase === 'finishing' ? 'Finishing up' : state?.progress.phase === 'preparing' ? 'Preparing files' : 'Copying application files'}</span><strong>{percent}%</strong></div>
        <div class="progress-track" role="progressbar" aria-label="Installation progress" aria-valuenow={percent} aria-valuemin="0" aria-valuemax="100"><div style:width="{percent}%"></div></div>
        <div class="progress-detail"><span>{bytes(state?.progress.copiedBytes ?? 0)} / {bytes(state?.progress.totalBytes ?? 0)}</span><span>Please keep this window open</span></div>
        <code class="current-file">{state?.progress.file || 'Preparing your installation…'}</code>
        <button class="secondary cancel" on:click={cancel} disabled={cancelRequested}>{cancelRequested ? 'Cancelling…' : 'Cancel installation'}</button>
      {:else}
        <span class="section-number">LET’S GET YOU SET UP</span><h1>Your space.<br />Your setup.</h1><p class="intro">Choose where {info.name} lives and how it starts.</p>
        <form on:submit|preventDefault={install}>
          <label class="field-label" for="install-directory">Installation folder</label>
          <div class="path-field"><span aria-hidden="true">⌁</span><input id="install-directory" bind:value={directory} required spellcheck="false" autocomplete="off" aria-describedby="directory-help" disabled={browsing} /><button type="button" class="browse" on:click={browse} disabled={browsing}>{browsing ? 'Choosing…' : 'Browse'}</button></div>
          <p id="directory-help" class="help">Use a new folder. Setup creates it for you.</p>
          {#if info.allowStartup}<label class="startup-option"><input type="checkbox" bind:checked={startup} /><span><strong>Start with your day</strong><small>Launch {info.name} when you sign in to your computer.</small></span><span class="option-icon" aria-hidden="true">↗</span></label>{/if}
          <div class="action-row"><span class="install-note">Just for this user.<br />No administrator access needed.</span><button type="submit" class="primary" disabled={!directory.trim() || browsing}>Install {info.name}<span aria-hidden="true">↗</span></button></div>
        </form>
      {/if}
      {#if error}<div class="error" role="alert">{error}</div>{/if}
      {#if state?.status === 'cancelled'}<p role="status" class="help">Installation cancelled. Choose your settings to try again.</p>{/if}
    </div>
    <footer><slot name="footer"><span>Made to feel at home.</span><span>POWERED BY WAILS INSTALLER</span></slot></footer>
  </div>
</section>

<style>
  .installer { --_accent:var(--installer-accent,#d3ef78); --_ink:var(--installer-ink,#202820); --_muted:var(--installer-muted,#687064); --_border:var(--installer-border,#dedfd6); --_surface:var(--installer-surface,#f9faf5); --_side:var(--installer-sidebar,#26382d); display:grid;grid-template-columns:300px minmax(0,1fr);min-height:670px;max-width:1020px;margin:0 auto;background:var(--_surface);color:var(--_ink);font-family:var(--installer-font,'Trebuchet MS',sans-serif);border:1px solid var(--_border);border-radius:var(--installer-radius,16px);overflow:hidden;box-sizing:border-box;line-height:1.5 }
  .installer :global(*) { box-sizing:border-box }
  .sidebar { background:var(--_side);color:var(--installer-sidebar-text,#eff4e9);padding:34px 30px 22px;display:flex;flex-direction:column;position:relative;overflow:hidden }
  .brand { display:flex;align-items:center;gap:12px;font-size:21px;font-weight:700;letter-spacing:-.6px }
  .brand small { display:block;font-size:8px;font-weight:400;letter-spacing:1.6px;margin-top:3px;opacity:.65 }
  .mark { width:39px;height:39px;border-radius:11px;background:var(--_accent);color:var(--_ink);font-size:29px;display:grid;place-items:center;font-weight:400 }
  .logo { width:39px;height:39px;object-fit:contain }
  .illustration { height:230px;position:relative;display:grid;place-items:center;margin-top:28px }
  .orbit { width:194px;height:194px;border:1px solid #ffffff20;border-radius:50%;position:absolute }
  .orbit::after { content:'';position:absolute;inset:19px;border:1px dashed #ffffff19;border-radius:50% }
  .package { height:94px;width:94px;border:1px solid var(--_accent);transform:rotate(-12deg);border-radius:16px;background:#ffffff06;display:grid;place-items:center;box-shadow:16px 16px 0 #ffffff06 }
  .package::after { content:'';position:absolute;top:0;left:37px;width:18px;height:32px;border:1px solid var(--_accent);border-top:0;opacity:.55 }
  .package span { font-size:38px;color:var(--_accent);padding-top:16px }
  .coordinate { position:absolute;bottom:7px;left:6px;font:9px monospace;letter-spacing:2px;opacity:.5 }
  h2 { font-size:32px;line-height:1.13;font-weight:400;letter-spacing:-1.4px;margin:26px 0 16px }
  .sidebar-description { font-size:12px;opacity:.7;max-width:210px;margin:0 0 35px;line-height:1.8 }
  .sidebar-bottom { margin-top:auto;padding-top:20px;border-top:1px solid #ffffff22;display:flex;align-items:center;gap:7px;font-size:9px;opacity:.75 }
  .status-dot { width:5px;height:5px;background:var(--_accent);border-radius:50% }
  .version { margin-left:auto;font-family:monospace }
  .main { padding:32px 42px 22px;display:flex;flex-direction:column;min-width:0 }
  header { display:flex;justify-content:space-between;align-items:center;gap:12px }
  .eyebrow { font-size:9px;letter-spacing:1.8px;color:var(--_muted) }
  .platform { font-size:10px;border:1px solid var(--_border);border-radius:20px;padding:3px 10px;text-transform:capitalize }
  nav { margin-top:31px;padding-bottom:23px;border-bottom:1px solid var(--_border) }
  ol { display:flex;list-style:none;padding:0;margin:0;gap:28px }
  li { display:flex;align-items:center;gap:8px;font-size:11px;color:var(--_muted) }
  li span { width:22px;height:22px;border:1px solid var(--_border);border-radius:50%;display:grid;place-items:center;font:9px monospace }
  li.active { color:var(--_ink);font-weight:700 }
  li.active span { background:var(--_ink);color:var(--_surface);border-color:var(--_ink) }
  li.done span { background:var(--_accent);border-color:var(--_accent);color:var(--_ink) }
  .content { padding:34px 0 30px;flex:1;min-width:0 }
  .section-number { font:9px monospace;letter-spacing:1.6px;color:var(--_muted) }
  h1 { font-size:var(--installer-heading-size,46px);font-weight:400;line-height:1.06;letter-spacing:-2.4px;margin:13px 0 17px }
  .intro { color:var(--_muted);font-size:12px;margin:0 0 29px }
  .field-label { display:block;font-size:12px;font-weight:700;margin-bottom:9px }
  .path-field { display:flex;align-items:center;gap:10px;border:1px solid var(--_border);border-radius:7px;background:var(--installer-input,#fff);padding:5px 6px 5px 12px }
  .path-field:focus-within { outline:2px solid var(--_ink);outline-offset:2px }
  .path-field > span { font-size:24px;color:var(--_muted) }
  input:not([type=checkbox]) { min-width:0;flex:1;border:0;background:transparent;color:var(--_ink);font:11px monospace;outline:none;padding:7px 0;width:100% }
  button { font-family:inherit;cursor:pointer;transition:background .15s,transform .15s }
  button:focus-visible,input[type=checkbox]:focus-visible { outline:2px solid var(--_ink);outline-offset:4px }
  button:disabled { opacity:.5;cursor:default }
  .browse { border:0;border-radius:4px;background:var(--_surface);color:var(--_ink);font-size:10px;padding:9px 12px }
  .help { font-size:10px;color:var(--_muted);margin:8px 0 23px }
  .startup-option { display:flex;align-items:center;gap:13px;border:1px solid var(--_border);border-radius:7px;padding:16px;cursor:pointer }
  input[type=checkbox] { width:17px;height:17px;accent-color:var(--_ink);flex-shrink:0;margin:0 }
  .startup-option strong { font-size:12px;font-weight:700;display:block }
  .startup-option small { font-size:10px;color:var(--_muted);display:block;margin-top:3px }
  .option-icon { margin-left:auto;font-size:20px;color:var(--_muted) }
  .action-row { display:flex;align-items:center;justify-content:space-between;gap:14px;margin-top:28px }
  .install-note { font-size:9px;line-height:1.7;color:var(--_muted) }
  .primary { display:flex;align-items:center;justify-content:space-between;gap:26px;padding:13px 17px;font-size:12px;font-weight:700;border:1px solid transparent;border-radius:6px;background:var(--_accent);color:var(--installer-button-text,#202820) }
  .primary:hover:not(:disabled) { filter:brightness(.96);transform:translateY(-1px) }
  .primary span { font-size:20px;font-weight:400;line-height:1 }
  footer { border-top:1px solid var(--_border);padding-top:17px;display:flex;justify-content:space-between;align-items:center;gap:16px;color:var(--_muted);font-size:9px }
  footer > span:last-child { font-size:7px;letter-spacing:1px }
  .error { font-size:12px;line-height:1.6;padding:13px 15px;background:var(--installer-error-bg,#fff0e9);color:var(--installer-error-text,#8b2f19);border:1px solid currentColor;border-radius:6px;margin-top:20px;overflow-wrap:anywhere }
  .loading { color:var(--_muted);padding:50px 0;font-size:14px }
  .progress-top { display:flex;justify-content:space-between;align-items:center;font-size:12px;margin-top:50px }
  .progress-top strong { font:24px monospace }
  .progress-track { height:8px;background:var(--_border);border-radius:10px;margin:15px 0 12px;overflow:hidden }
  .progress-track > div { height:100%;background:var(--_ink);transition:width .2s }
  .progress-detail { display:flex;justify-content:space-between;gap:12px;font-size:10px;color:var(--_muted) }
  .current-file { font-size:10px;display:block;margin-top:25px;overflow-wrap:anywhere;color:var(--_muted) }
  .secondary { border:1px solid var(--_border);border-radius:6px;padding:10px 15px;background:transparent;color:var(--_ink);font-size:11px }
  .cancel { margin-top:32px }
  .success-icon { background:var(--_accent);color:var(--_ink);border-radius:50%;width:40px;height:40px;display:grid;place-items:center;font-size:22px;margin-bottom:22px }
  .receipt { border-block:1px solid var(--_border);padding:16px 0;margin-bottom:24px }
  .receipt > span { font:8px monospace;letter-spacing:1px;color:var(--_muted) }
  .receipt code { display:block;font-size:11px;margin-top:8px;overflow-wrap:anywhere }
  .receipt p,.launch-note { font-size:11px;color:var(--_muted) }
  @media(max-width:780px) { .installer { grid-template-columns:220px minmax(0,1fr) }.sidebar { padding:28px 22px }.main { padding:28px 25px 20px }ol { gap:16px }h1 { font-size:39px }.action-row { flex-wrap:wrap } }
  @media(max-width:600px) { .installer { grid-template-columns:1fr;border-radius:10px;min-height:0 }.sidebar { padding:20px 24px }.illustration,h2,.sidebar-description,.sidebar-bottom { display:none }.main { padding:25px 24px 20px }nav { margin-top:24px }ol { justify-content:space-between;gap:12px }.content { padding-top:28px }.action-row { gap:20px }.primary { flex:1 }footer { flex-wrap:wrap } }
  @media(prefers-reduced-motion:reduce) { button,.progress-track > div { transition:none }.primary:hover:not(:disabled) { transform:none } }
</style>
