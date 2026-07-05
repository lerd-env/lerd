<script lang="ts">
  import PortRow from './PortRow.svelte';
  import PortsEditor from '$components/PortsEditor.svelte';
  import { type Service, setServicePorts } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  const isBuiltin = $derived(Boolean(svc.preset_owned));

  // Number inputs bound with bind:value yield number | null (null when empty),
  // so these stay numeric — never strings.
  let publishedInput = $state<number | null>(null);
  let secondaryInputs = $state<Record<number, number | null>>({});
  let extraPorts = $state<string[]>([]);
  let saving = $state(false);
  let error = $state('');

  // The seed a service would produce. `published` mirrors the modal's fallback
  // chain so an unset override reads as the preset default, not a blank field.
  function seed(s: Service) {
    const secondary: Record<number, number | null> = {};
    for (const p of s.secondary_ports ?? []) secondary[p.container] = p.published || p.default;
    return {
      published: s.published_port || s.default_port || null,
      secondary,
      extra: [...(s.extra_ports ?? [])]
    };
  }

  // Pin the reseed to the service name only. Every services WebSocket broadcast
  // passes a fresh svc object even when the name is unchanged; reseeding on the
  // object would clobber in-progress edits on every push.
  const currentName = $derived(svc.name);
  $effect(() => {
    currentName;
    const s = seed(svc);
    publishedInput = s.published;
    secondaryInputs = s.secondary;
    extraPorts = [...s.extra];
    saving = false;
    error = '';
  });

  // Live baseline off the current svc so a successful save (which updates svc
  // via the broadcast) settles dirty back to false without an extra reseed.
  const baseline = $derived(seed(svc));
  const dirty = $derived.by(() => {
    if ((publishedInput ?? null) !== (baseline.published ?? null)) return true;
    for (const p of svc.secondary_ports ?? []) {
      if ((secondaryInputs[p.container] ?? null) !== (baseline.secondary[p.container] ?? null)) return true;
    }
    const cur = [...extraPorts].sort();
    const base = [...baseline.extra].sort();
    if (cur.length !== base.length) return true;
    return cur.some((v, i) => v !== base[i]);
  });

  function validPort(n: number | null): n is number {
    return n != null && Number.isInteger(n) && n >= 0 && n <= 65535;
  }

  function revert() {
    const s = seed(svc);
    publishedInput = s.published;
    secondaryInputs = s.secondary;
    extraPorts = [...s.extra];
    error = '';
  }

  async function save() {
    error = '';
    let published: number | null = null;
    if (publishedInput != null) {
      if (!validPort(publishedInput)) {
        error = m.services_ports_invalidPort();
        return;
      }
      // The preset default isn't an override, so saving it leaves the field
      // unset and keeps the auto-shift guard on.
      published = svc.default_port && publishedInput === svc.default_port ? null : publishedInput;
    }
    // Secondary mappings: send every field keyed by container port. A blanked
    // input means "reset to default", so send the preset default, which the
    // backend normalises back to "no override" and thus clears an existing one.
    const publishedPorts: Record<string, number> = {};
    for (const p of svc.secondary_ports ?? []) {
      const v = secondaryInputs[p.container] ?? p.default;
      if (!validPort(v)) {
        error = m.services_ports_invalidPort();
        return;
      }
      publishedPorts[String(p.container)] = v;
    }
    saving = true;
    try {
      const res = await setServicePorts(svc.name, {
        published_port: published,
        published_ports: publishedPorts,
        extra_ports: isBuiltin ? extraPorts : []
      });
      if (!res.ok) {
        error = res.error || m.common_failed();
        return;
      }
    } finally {
      saving = false;
    }
  }
</script>

<div class="flex flex-col h-full">
  <div class="sticky top-0 z-10">
    <div class="flex items-center justify-between bg-gray-50 dark:bg-white/3 px-3 py-1.5 border-b border-gray-200 dark:border-lerd-border">
      <div class="flex items-center gap-2 min-w-0">
        {#if dirty && !saving}
          <span class="text-[10px] font-medium text-amber-600 dark:text-amber-400">{m.tuningEditor_unsaved()}</span>
        {/if}
      </div>
      <div class="flex items-center gap-2 shrink-0">
        {#if dirty}
          <button
            type="button"
            onclick={revert}
            disabled={saving}
            class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-40"
          >
            {m.tuningEditor_revert()}
          </button>
          <button
            type="button"
            onclick={save}
            disabled={saving}
            class="text-xs px-3 py-1 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-white transition-colors disabled:opacity-40"
          >
            {saving ? m.services_ports_applying() : m.common_save()}
          </button>
        {/if}
      </div>
    </div>
  </div>

  <div class="flex-1 overflow-y-auto p-3 sm:p-5 space-y-5">
    <div class="space-y-2">
      <div>
        <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
          {m.services_ports_publishedLabel()}
        </span>
        <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
          {m.services_ports_publishedHelp({ name: svc.name })}
        </p>
      </div>
      <PortRow bind:value={publishedInput} defaultPort={svc.default_port} disabled={saving} onenter={save} />
      {#each svc.secondary_ports ?? [] as p (p.container)}
        <PortRow bind:value={secondaryInputs[p.container]} defaultPort={p.default} disabled={saving} onenter={save} />
      {/each}
    </div>

    <div class="space-y-2 border-t border-gray-100 dark:border-lerd-border pt-4">
      <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
        {m.services_ports_extraTitle()}
      </span>
      {#if !isBuiltin}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.services_ports_extraPresetOnly()}</p>
      {:else}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.services_ports_extraHelp()}</p>
        <PortsEditor
          ports={extraPorts}
          disabled={saving}
          empty={m.services_ports_extraEmpty()}
          onadd={(spec) => (extraPorts = [...extraPorts, spec])}
          onremove={(spec) => (extraPorts = extraPorts.filter((p) => p !== spec))}
        />
      {/if}
    </div>

    {#if error}
      <p class="text-xs text-red-500">{error}</p>
    {/if}
  </div>
</div>
