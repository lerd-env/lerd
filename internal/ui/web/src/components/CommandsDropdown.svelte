<script lang="ts">
  import { m } from '../paraglide/messages.js';
  import { loadCommands, launchCommand, runningName, type Command } from '$stores/commands';

  interface Props {
    domain: string;
    branch?: string;
  }
  let { domain, branch = '' }: Props = $props();

  let commands: Command[] = $state([]);
  let menuOpen = $state(false);
  let triggerEl: HTMLButtonElement | null = $state(null);
  let menuPos = $state({ top: 0, left: 0, width: 288 });

  async function refresh() {
    if (!domain) return;
    try {
      commands = await loadCommands(domain, branch);
    } catch {
      commands = [];
    }
  }

  // Initial load on mount, plus a refresh whenever the domain or branch prop changes.
  $effect(() => {
    void domain;
    void branch;
    void refresh();
  });

  function toggleMenu() {
    if (menuOpen) {
      menuOpen = false;
      return;
    }
    if (!triggerEl) return;
    // Refresh in the background so external edits (MCP command_add, manual
    // .lerd.yaml edits since mount) show up the moment the menu opens.
    // Fire-and-forget; the menu opens with whatever's cached and updates
    // when the fetch resolves.
    void refresh();
    const r = triggerEl.getBoundingClientRect();
    const margin = 8;
    // On narrow viewports the 288px desktop width would overflow; cap to
    // the viewport minus margins. Otherwise use the preferred width and
    // right-clamp if anchoring at the trigger would overflow.
    const desired = 288;
    const maxWidth = Math.min(desired, window.innerWidth - margin * 2);
    let left = r.left;
    if (left + maxWidth + margin > window.innerWidth) {
      left = Math.max(margin, window.innerWidth - maxWidth - margin);
    }
    menuPos = { top: r.bottom + 4, left, width: maxWidth };
    menuOpen = true;
  }

  function pick(cmd: Command) {
    menuOpen = false;
    launchCommand(domain, cmd, { branch });
  }

  function iconPath(name?: string): string {
    switch (name) {
      case 'broom': return 'M4 20l6-6m4-4l6-6m-2 2l-2-2m-4 14l-2-2m-2-6l8 8';
      case 'database': return 'M4 7c0-1.66 3.58-3 8-3s8 1.34 8 3-3.58 3-8 3-8-1.34-8-3zm0 0v10c0 1.66 3.58 3 8 3s8-1.34 8-3V7M4 12c0 1.66 3.58 3 8 3s8-1.34 8-3';
      case 'refresh': return 'M4 4v5h5M20 20v-5h-5M20.49 9A9 9 0 005.64 5.64L4 4m16 16l-1.64-1.64A9 9 0 014.51 15';
      case 'link': return 'M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71';
      case 'check': return 'M5 13l4 4L19 7';
      case 'list': return 'M4 6h16M4 12h16M4 18h16';
      case 'key': return 'M21 2l-9.5 9.5M15.5 7.5L19 11M9 12a4 4 0 11-4-4 4 4 0 014 4z';
      case 'edit': return 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.41-9.41a2 2 0 112.83 2.83L11.83 15H9v-2.83l8.59-8.58z';
      case 'arrow-down': return 'M19 14l-7 7-7-7M12 4v17';
      case 'arrow-up': return 'M5 10l7-7 7 7M12 3v18';
      case 'play': return 'M5 3l14 9-14 9V3z';
      case 'terminal': return 'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z';
      default: return 'M13 10V3L4 14h7v7l9-11h-7z';
    }
  }

  function handleDocClick(e: MouseEvent) {
    if (!menuOpen) return;
    const t = e.target as Node;
    if (triggerEl && (triggerEl === t || triggerEl.contains(t))) return;
    const menu = document.getElementById('cmds-dropdown-menu');
    if (menu && menu.contains(t)) return;
    menuOpen = false;
  }

  function handleScroll(e: Event) {
    if (!menuOpen) return;
    // Wheel-scrolling inside the menu (overflow-y-auto on a long command
    // list) bubbles up through capture and would otherwise close the menu
    // on the first tick. Only close when the surrounding page or an
    // ancestor scrolls — that's the case the close-on-scroll exists for,
    // since the menu is position:fixed and would detach from the trigger.
    const t = e.target as Node | null;
    const menu = document.getElementById('cmds-dropdown-menu');
    if (menu && t && menu.contains(t)) return;
    menuOpen = false;
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && menuOpen) {
      e.preventDefault();
      menuOpen = false;
    }
  }

  $effect(() => {
    document.addEventListener('click', handleDocClick);
    window.addEventListener('scroll', handleScroll, true);
    window.addEventListener('resize', handleScroll);
    window.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('click', handleDocClick);
      window.removeEventListener('scroll', handleScroll, true);
      window.removeEventListener('resize', handleScroll);
      window.removeEventListener('keydown', handleKey);
    };
  });

  const hasCommands = $derived(commands.length > 0);
</script>

{#if hasCommands}
<div class="relative inline-block">
  <button
    bind:this={triggerEl}
    type="button"
    onclick={toggleMenu}
    disabled={$runningName !== null}
    title=""
    class="inline-flex items-center gap-1.5 h-7 px-2.5 rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card hover:border-lerd-red hover:text-lerd-red transition-colors text-xs font-medium text-gray-700 dark:text-gray-200 disabled:opacity-40"
  >
    {#if $runningName}
      <svg class="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
      </svg>
      <span>{m.cmd_running()}</span>
    {:else}
      <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
      </svg>
      <span>{m.cmd_commands()}</span>
      <svg class="w-3 h-3 ml-0.5 transition-transform {menuOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
      </svg>
    {/if}
  </button>

  {#if menuOpen}
    <div
      id="cmds-dropdown-menu"
      style="position: fixed; top: {menuPos.top}px; left: {menuPos.left}px; width: {menuPos.width}px;"
      class="z-50 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl ring-1 ring-black/5 py-1 max-h-96 overflow-y-auto"
    >
      {#each commands as c (c.name)}
        <button
          type="button"
          onclick={() => pick(c)}
          title={(c.description ?? '') + (c.description ? '\n\n' : '') + '$ ' + c.command}
          class="group w-full flex items-start gap-2.5 px-3 py-2 hover:bg-gray-50 dark:hover:bg-white/5 text-left transition-colors"
        >
          <span class="shrink-0 mt-0.5 w-5 h-5 rounded bg-gray-100 dark:bg-white/5 flex items-center justify-center text-gray-500 dark:text-gray-400 group-hover:text-lerd-red">
            <svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" d={iconPath(c.icon)} />
            </svg>
          </span>
          <span class="flex-1 min-w-0">
            <span class="flex items-center gap-1.5">
              <span class="text-xs font-medium text-gray-900 dark:text-gray-100">{c.label || c.name}</span>
              {#if c.confirm}
                <span class="shrink-0 w-1.5 h-1.5 rounded-full bg-amber-500" title={m.cmd_asksBeforeRunning()}></span>
              {/if}
            </span>
            <span class="block text-[10px] text-gray-500 dark:text-gray-400 font-mono truncate">{c.command}</span>
          </span>
        </button>
      {/each}
    </div>
  {/if}
</div>
{/if}
