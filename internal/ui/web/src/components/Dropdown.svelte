<script lang="ts" module>
  export interface DropdownOption {
    value: string;
    label?: string;       // display text; falls back to value
    description?: string; // dim secondary line under label
    disabled?: boolean;
  }
</script>

<script lang="ts">
  interface Props {
    value: string;
    options: Array<string | DropdownOption>;
    onchange: (v: string) => void;
    label?: string;          // optional prefix in trigger ("PHP 8.3" etc); also prefixes menu entries
    placeholder?: string;    // shown when value is empty
    disabled?: boolean;
    title?: string;          // tooltip on trigger
    inherited?: boolean;     // dashed violet border (used by PHP/Node version pickers)
    inheritedSuffix?: string;
    width?: 'auto' | 'full'; // trigger sizing; 'full' fills container (form-style)
    minMenuWidth?: number;   // px; default 160
    align?: 'left' | 'right';
  }
  let {
    value,
    options,
    onchange,
    label = '',
    placeholder = '',
    disabled = false,
    title = '',
    inherited = false,
    inheritedSuffix = '',
    width = 'auto',
    minMenuWidth = 160,
    align = 'left'
  }: Props = $props();

  let open = $state(false);
  let triggerEl: HTMLButtonElement | null = $state(null);
  let menuEl: HTMLDivElement | null = $state(null);
  let menuPos = $state({ top: 0, left: 0, width: 160 });
  let highlighted = $state(-1);
  let typeahead = $state('');
  let typeaheadTimer: ReturnType<typeof setTimeout> | null = null;
  const menuId = `dd-${Math.random().toString(36).slice(2, 9)}`;

  function normalize(opts: typeof options): DropdownOption[] {
    return opts.map((o) =>
      typeof o === 'string' ? { value: o, label: o } : { ...o, label: o.label ?? o.value }
    );
  }
  const normalized = $derived(normalize(options));

  function displayFor(opt: DropdownOption): string {
    return label ? label + ' ' + (opt.label || opt.value) : (opt.label || opt.value);
  }

  const selectedIdx = $derived(normalized.findIndex((o) => o.value === value));
  const display = $derived.by(() => {
    if (!value) return placeholder || label || 'Select…';
    if (selectedIdx >= 0) return displayFor(normalized[selectedIdx]);
    return value;
  });

  function openMenu() {
    if (!triggerEl) return;
    const r = triggerEl.getBoundingClientRect();
    const margin = 8;
    const desired = Math.max(minMenuWidth, r.width);
    const max = Math.min(desired, window.innerWidth - margin * 2);
    let left = align === 'right' ? r.right - max : r.left;
    if (left + max + margin > window.innerWidth) left = Math.max(margin, window.innerWidth - max - margin);
    if (left < margin) left = margin;
    menuPos = { top: r.bottom + 4, left, width: max };
    highlighted = selectedIdx >= 0 ? selectedIdx : 0;
    open = true;
    queueMicrotask(scrollHighlightedIntoView);
  }

  function closeMenu() {
    open = false;
    highlighted = -1;
    typeahead = '';
    if (typeaheadTimer) {
      clearTimeout(typeaheadTimer);
      typeaheadTimer = null;
    }
  }

  function toggle() {
    if (open) closeMenu();
    else openMenu();
  }

  function pick(opt: DropdownOption) {
    closeMenu();
    if (opt.disabled) return;
    if (opt.value !== value) onchange(opt.value);
  }

  function scrollHighlightedIntoView() {
    if (!menuEl) return;
    const node = menuEl.querySelector<HTMLElement>('[data-highlighted="true"]');
    // JSDOM doesn't implement scrollIntoView; guard so tests don't blow up.
    if (node && typeof node.scrollIntoView === 'function') node.scrollIntoView({ block: 'nearest' });
  }

  function moveHighlight(delta: number) {
    if (!normalized.length) return;
    let next = highlighted < 0 ? 0 : highlighted + delta;
    const n = normalized.length;
    next = ((next % n) + n) % n;
    // Skip disabled entries.
    let safety = n;
    while (normalized[next].disabled && safety-- > 0) {
      next = ((next + delta) % n + n) % n;
    }
    highlighted = next;
    scrollHighlightedIntoView();
  }

  function applyTypeahead(ch: string) {
    if (typeaheadTimer) clearTimeout(typeaheadTimer);
    typeahead = (typeahead + ch).toLowerCase();
    typeaheadTimer = setTimeout(() => (typeahead = ''), 600);
    const match = normalized.findIndex(
      (o) => !o.disabled && (o.label || o.value).toLowerCase().startsWith(typeahead)
    );
    if (match >= 0) {
      highlighted = match;
      scrollHighlightedIntoView();
    }
  }

  function onTriggerKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      // Stop the event from bubbling to the document-level menu handler,
      // which would double-handle and advance the highlight again.
      e.stopPropagation();
      if (!open) openMenu();
      if (e.key === 'ArrowDown') moveHighlight(1);
      else if (e.key === 'ArrowUp') moveHighlight(-1);
    } else if (e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey) {
      e.stopPropagation();
      if (!open) openMenu();
      applyTypeahead(e.key);
    }
  }

  function onMenuKey(e: KeyboardEvent) {
    if (!open) return;
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        closeMenu();
        triggerEl?.focus();
        break;
      case 'ArrowDown':
        e.preventDefault();
        moveHighlight(1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        moveHighlight(-1);
        break;
      case 'Home':
        e.preventDefault();
        highlighted = 0;
        scrollHighlightedIntoView();
        break;
      case 'End':
        e.preventDefault();
        highlighted = normalized.length - 1;
        scrollHighlightedIntoView();
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        if (highlighted >= 0 && highlighted < normalized.length) pick(normalized[highlighted]);
        break;
      case 'Tab':
        closeMenu();
        break;
      default:
        if (e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey) {
          applyTypeahead(e.key);
        }
    }
  }

  function handleDocClick(e: MouseEvent) {
    if (!open) return;
    const t = e.target as Node;
    if (triggerEl && (triggerEl === t || triggerEl.contains(t))) return;
    if (menuEl && menuEl.contains(t)) return;
    closeMenu();
  }

  function handleScroll(e: Event) {
    if (!open) return;
    // Wheel-scrolling inside the menu (overflow-y-auto on a long option
    // list) bubbles up through capture and would otherwise close the menu
    // on the first tick. Leave the menu open for those; only close when
    // the surrounding page or an ancestor scrolls — that's the case the
    // close-on-scroll exists for, since the menu is position:fixed and
    // would detach from the trigger.
    const t = e.target as Node | null;
    if (menuEl && t && menuEl.contains(t)) return;
    closeMenu();
  }

  $effect(() => {
    document.addEventListener('click', handleDocClick);
    document.addEventListener('keydown', onMenuKey);
    window.addEventListener('scroll', handleScroll, true);
    window.addEventListener('resize', handleScroll);
    return () => {
      document.removeEventListener('click', handleDocClick);
      document.removeEventListener('keydown', onMenuKey);
      window.removeEventListener('scroll', handleScroll, true);
      window.removeEventListener('resize', handleScroll);
      if (typeaheadTimer) clearTimeout(typeaheadTimer);
    };
  });
</script>

<div class="relative inline-block {width === 'full' ? 'w-full' : ''}">
  <button
    bind:this={triggerEl}
    type="button"
    onclick={toggle}
    onkeydown={onTriggerKey}
    {disabled}
    {title}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-controls={menuId}
    class="{width === 'full' ? 'w-full' : ''} inline-flex items-center justify-between gap-1.5 h-7 px-2.5 rounded-md border bg-white dark:bg-lerd-card hover:border-lerd-red hover:text-lerd-red transition-colors text-xs font-medium text-gray-700 dark:text-gray-200 disabled:opacity-50 disabled:cursor-not-allowed {inherited ? 'border-dashed border-violet-300 dark:border-violet-700' : 'border-gray-200 dark:border-lerd-border'}"
  >
    <span class="truncate text-left">{display}</span>
    <svg class="w-3 h-3 ml-0.5 shrink-0 transition-transform {open ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
    </svg>
  </button>

  {#if open}
    <div
      bind:this={menuEl}
      id={menuId}
      role="listbox"
      style="position: fixed; top: {menuPos.top}px; left: {menuPos.left}px; width: {menuPos.width}px;"
      class="z-50 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl ring-1 ring-black/5 py-1 max-h-72 overflow-y-auto"
    >
      {#each normalized as opt, i (opt.value + ':' + i)}
        {@const selected = opt.value === value}
        {@const isHighlighted = i === highlighted}
        <button
          type="button"
          role="option"
          aria-selected={selected}
          data-highlighted={isHighlighted}
          disabled={opt.disabled}
          onclick={() => pick(opt)}
          onmouseenter={() => (highlighted = i)}
          class="w-full flex items-start gap-2 px-3 py-1.5 text-left text-xs transition-colors disabled:opacity-40 disabled:cursor-not-allowed {isHighlighted ? 'bg-gray-100 dark:bg-white/10' : 'hover:bg-gray-50 dark:hover:bg-white/5'} {selected ? 'text-lerd-red font-semibold' : 'text-gray-700 dark:text-gray-200'}"
        >
          <span class="shrink-0 w-3 h-3 mt-0.5">
            {#if selected}
              <svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
            {/if}
          </span>
          <span class="flex-1 min-w-0">
            <span class="block truncate">{displayFor(opt)}{inherited && selected && inheritedSuffix ? ' ' + inheritedSuffix : ''}</span>
            {#if opt.description}
              <span class="block text-[10px] text-gray-500 dark:text-gray-400 truncate font-normal">{opt.description}</span>
            {/if}
          </span>
        </button>
      {/each}
    </div>
  {/if}
</div>
