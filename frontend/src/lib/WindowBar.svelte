<script lang="ts">
  /** Window actions belong to the host so this component works with Wails v2 or v3. */
  export let title = 'Installer';
  export let onClose: () => void;
  export let onMinimize: (() => void) | undefined = undefined;
</script>

<header class="window-bar">
  <span class="title">{title}</span>
  <div class="controls">
    {#if onMinimize}
      <button type="button" aria-label="Minimize window" on:click={onMinimize}>
        <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8h10" /></svg>
      </button>
    {/if}
    <button type="button" class="close" aria-label="Close window" on:click={onClose}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8m0-8-8 8" /></svg>
    </button>
  </div>
</header>

<style>
  .window-bar { --wails-draggable:drag;display:flex;align-items:center;justify-content:space-between;flex-shrink:0;min-height:38px;background:var(--installer-surface,#f9faf5);color:var(--installer-ink,#202820);font:12px var(--installer-font,'Trebuchet MS',sans-serif);user-select:none }
  .title { padding:0 16px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap }
  .controls { --wails-draggable:no-drag;display:flex;align-self:stretch;flex-shrink:0 }
  button { --wails-draggable:no-drag;display:grid;place-items:center;width:44px;min-height:38px;border:0;border-radius:0;background:transparent;color:inherit;cursor:pointer }
  button:hover { background:var(--installer-border,#dedfd6) }
  button:focus-visible { outline:2px solid currentColor;outline-offset:-4px }
  .close:hover { background:var(--installer-error-bg,#fff0e9);color:var(--installer-error-text,#8b2f19) }
  svg { width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:1.3;pointer-events:none }
</style>
