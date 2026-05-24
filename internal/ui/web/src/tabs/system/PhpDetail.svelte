<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Toggle from '$components/Toggle.svelte';
  import InfoRow from '$components/InfoRow.svelte';
  import LogViewer from '$components/LogViewer.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import { status, loadStatus, fpmRunning } from '$stores/status';
  import { setDefaultPhp, startPhp, stopPhp, removePhp } from '$stores/phpVersions';
  import {
    phpExtensions,
    loadPhpExtensions,
    addPhpExtension,
    removePhpExtension
  } from '$stores/phpExtensions';
  import { sites, sitesByPhp } from '$stores/sites';
  import { xdebugOn, xdebugOff, XDEBUG_MODES, type XdebugMode } from '$stores/xdebug';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();

  // Reload custom extensions whenever the PHP tab swaps version.
  $effect(() => {
    loadPhpExtensions(version);
  });

  const customExts = $derived($phpExtensions[version] ?? []);

  let extAdding = $state(false);
  let extError = $state('');
  let extName = $state('');
  let extApkDeps = $state('');
  let removingExt = $state(''); // name of ext currently being removed (for per-row spinner)

  async function onAddExtension() {
    const name = extName.trim();
    if (!name) {
      extError = 'Informe o nome da extensão (ex: imap, swoole, sqlsrv)';
      return;
    }
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
      extError = 'Nome inválido — use apenas letras, dígitos, hífens e sublinhados';
      return;
    }
    extAdding = true;
    extError = '';
    const deps = extApkDeps
      .split(/[\s,]+/)
      .map((d) => d.trim())
      .filter(Boolean);
    try {
      const res = await addPhpExtension(version, name, deps);
      if (res.ok) {
        extName = '';
        extApkDeps = '';
        if (res.error) {
          // Soft warning (installed but FPM restart failed, etc.)
          extError = res.error;
        }
      } else {
        extError = res.error || 'Falha ao instalar a extensão';
      }
    } finally {
      extAdding = false;
    }
  }

  async function onRemoveExtension(name: string) {
    if (!confirm(`Remover a extensão "${name}" do PHP ${version}? A imagem será reconstruída.`)) {
      return;
    }
    removingExt = name;
    extError = '';
    try {
      const res = await removePhpExtension(version, name);
      if (!res.ok) {
        extError = res.error || 'Falha ao remover a extensão';
      }
    } finally {
      removingExt = '';
    }
  }

  const running = $derived(fpmRunning(version));
  const isDefault = $derived($status.php_default === version);
  const siteCount = $derived($sitesByPhp.get(version) ?? 0);
  const fpm = $derived($status.php_fpms.find((f) => f.version === version));
  const xdebugEnabled = $derived(Boolean(fpm?.xdebug_enabled));
  const xdebugMode = $derived<XdebugMode>((fpm?.xdebug_mode as XdebugMode) || 'debug');
  const container = $derived('lerd-php' + version.replace('.', '') + '-fpm');
  const sitesUsing = $derived($sites.filter((s) => s.php_version === version));

  let defaultBusy = $state(false);
  let fpmBusy = $state(false);
  let removeBusy = $state(false);
  let xdebugBusy = $state(false);
  let removeError = $state('');

  async function onSetDefault() {
    defaultBusy = true;
    try {
      await setDefaultPhp(version);
      await loadStatus();
    } finally {
      defaultBusy = false;
    }
  }

  async function onToggleFpm() {
    fpmBusy = true;
    try {
      await (running ? stopPhp(version) : startPhp(version));
      await loadStatus();
    } finally {
      fpmBusy = false;
    }
  }

  async function onRemove() {
    removeBusy = true;
    removeError = '';
    try {
      const r = await removePhp(version);
      if (!r) removeError = m.common_failed();
      await loadStatus();
    } finally {
      removeBusy = false;
    }
  }

  async function onToggleXdebug() {
    xdebugBusy = true;
    try {
      if (xdebugEnabled) {
        await xdebugOff(version);
      } else {
        await xdebugOn(version, xdebugMode);
      }
      await loadStatus();
    } finally {
      xdebugBusy = false;
    }
  }

  async function onSetXdebugMode(e: Event) {
    const mode = (e.target as HTMLSelectElement).value as XdebugMode;
    if (mode === xdebugMode) return;
    xdebugBusy = true;
    try {
      await xdebugOn(version, mode);
      await loadStatus();
    } finally {
      xdebugBusy = false;
    }
  }
</script>

<DetailPanel>
  <div
    class="flex flex-wrap items-center justify-between gap-y-2 px-3 sm:px-5 py-4 border-b border-gray-100 dark:border-lerd-border shrink-0"
  >
    <div class="flex items-center gap-3">
      <span class="font-semibold text-gray-900 dark:text-white text-base">PHP {version}</span>
      <StatusPill tone={running ? 'ok' : 'muted'} label={running ? m.common_running() : m.common_stopped()} />
      {#if siteCount > 0}
        <span class="text-xs text-gray-400 dark:text-gray-500">
          {siteCount} {siteCount === 1 ? m.common_site() : m.common_sites()}
        </span>
      {/if}
    </div>
    <div class="flex items-center gap-2">
      {#if !isDefault}
        <DetailButton onclick={onSetDefault} disabled={defaultBusy} loading={defaultBusy}>
          {m.system_php_setDefault()}
        </DetailButton>
      {/if}
      {#if !isDefault}
        {#if running}
          <DetailButton
            onclick={onToggleFpm}
            disabled={fpmBusy}
            loading={fpmBusy}
            title={siteCount > 0 ? m.system_php_stopWarn({ count: siteCount }) : m.system_php_stopTitle()}
          >{m.common_stop()}</DetailButton>
        {:else}
          <DetailButton
            tone="success"
            onclick={onToggleFpm}
            disabled={fpmBusy}
            loading={fpmBusy}
            title={m.system_php_startTitle()}
          >{m.common_start()}</DetailButton>
        {/if}
        <DetailButton
          tone="danger"
          onclick={onRemove}
          disabled={removeBusy}
          loading={removeBusy}
          title={siteCount > 0 ? m.system_php_removeWarn({ count: siteCount }) : m.system_php_removeTitle()}
        >{m.common_remove()}</DetailButton>
      {/if}
    </div>
  </div>

  <div class="px-3 sm:px-5 py-3 space-y-4 shrink-0">
    <div class="flex items-center justify-between">
      <div>
        <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{m.system_php_xdebug()}</p>
        <p class="text-xs text-gray-400 mt-0.5">{m.system_php_xdebugHint()}</p>
      </div>
      <div class="flex items-center gap-2">
        {#if xdebugEnabled}
          <Dropdown
            value={xdebugMode}
            options={XDEBUG_MODES}
            disabled={xdebugBusy}
            title={m.system_php_xdebugModeTitle()}
            onchange={(v) => onSetXdebugMode({ target: { value: v } } as unknown as Event)}
          />
        {/if}
        <Toggle
          on={xdebugEnabled}
          tone="violet"
          loading={xdebugBusy}
          onclick={onToggleXdebug}
          title={xdebugEnabled ? 'Disable Xdebug' : 'Enable Xdebug'}
        />
      </div>
    </div>

    <InfoRow label={m.system_container()} value={container} />

    <!--
      Extensões customizadas — gerencia o equivalente de `lerd php:ext add/remove`
      pelo dashboard. O backend roda `podman build` + restart do FPM unit a cada
      add/remove (1–3 minutos), por isso o spinner fica de pé durante toda a
      operação e a UI bloqueia novos cliques.
    -->
    <div>
      <div class="flex items-center justify-between mb-2">
        <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Extensões customizadas</p>
        <span class="text-[10px] text-gray-400">{customExts.length} configurada{customExts.length === 1 ? '' : 's'}</span>
      </div>

      {#if customExts.length === 0}
        <p class="text-xs text-gray-400 mb-3">Nenhuma extensão extra além das já compiladas na imagem.</p>
      {:else}
        <div class="space-y-1.5 mb-3">
          {#each customExts as ext (ext.name)}
            <div class="flex items-center justify-between gap-2 px-2.5 py-1.5 bg-gray-50 dark:bg-white/5 border border-gray-200 dark:border-lerd-border rounded text-xs">
              <div class="flex items-center gap-2 min-w-0">
                <span class="font-mono font-medium text-gray-700 dark:text-gray-200 shrink-0">{ext.name}</span>
                {#if ext.apk_deps && ext.apk_deps.length > 0}
                  <span class="text-gray-400 truncate" title="Pacotes Alpine">apk: {ext.apk_deps.join(' ')}</span>
                {/if}
              </div>
              <button
                onclick={() => onRemoveExtension(ext.name)}
                disabled={removingExt === ext.name || extAdding}
                class="text-red-600 dark:text-red-400 hover:underline disabled:opacity-50 disabled:no-underline shrink-0"
                title="Remover extensão (reconstrói a imagem)"
              >
                {removingExt === ext.name ? 'removendo…' : 'remover'}
              </button>
            </div>
          {/each}
        </div>
      {/if}

      <div class="space-y-2">
        <div class="grid grid-cols-1 sm:grid-cols-[1fr_2fr_auto] gap-2">
          <input
            type="text"
            placeholder="ext (ex: imap)"
            bind:value={extName}
            disabled={extAdding || removingExt !== ''}
            class="font-mono text-xs px-2.5 py-1.5 bg-white dark:bg-lerd-dark-2 border border-gray-200 dark:border-lerd-border rounded text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-emerald-500 disabled:opacity-50"
          />
          <input
            type="text"
            placeholder="apk deps opcionais — ex: imap-dev krb5-dev openssl-dev"
            bind:value={extApkDeps}
            disabled={extAdding || removingExt !== ''}
            class="font-mono text-xs px-2.5 py-1.5 bg-white dark:bg-lerd-dark-2 border border-gray-200 dark:border-lerd-border rounded text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-emerald-500 disabled:opacity-50"
          />
          <DetailButton
            tone="success"
            onclick={onAddExtension}
            disabled={extAdding || removingExt !== '' || !extName.trim()}
            loading={extAdding}
            title="Instala via pecl/docker-php-ext-install, reconstrói a imagem e reinicia o FPM (1–3min)"
          >Adicionar</DetailButton>
        </div>
        <p class="text-[10px] text-gray-400 leading-relaxed">
          Equivalente a <code class="font-mono">lerd php:ext add &lt;ext&gt; {version} --apk-deps "&lt;pacotes&gt;"</code>.
          A imagem PHP {version} é reconstruída e o container reinicia ao final.
        </p>
      </div>

      {#if extError}
        <div class="mt-2 text-xs font-medium text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-500/10 rounded-lg px-3 py-2 break-words">
          {extError}
        </div>
      {/if}
    </div>

    <div>
      <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">{m.system_php_sites()}</p>
      {#if sitesUsing.length === 0}
        <p class="text-sm text-gray-400">{m.system_noSitesUsingPhp({ version })}</p>
      {:else}
        <div class="flex flex-wrap gap-2">
          {#each sitesUsing as s (s.domain)}
            <button
              onclick={() => goToTab('sites', s.domain)}
              class="inline-flex items-center gap-1.5 text-xs font-medium bg-gray-100 dark:bg-white/5 hover:bg-gray-200 dark:hover:bg-white/10 border border-gray-200 dark:border-lerd-border text-gray-700 dark:text-gray-300 rounded-full px-2.5 py-1 transition-colors"
            >
              <span class="w-1.5 h-1.5 rounded-full shrink-0 {s.fpm_running ? 'bg-emerald-500' : 'bg-gray-400'}"></span>
              {s.domain}
            </button>
          {/each}
        </div>
      {/if}
    </div>

    {#if removeError}
      <div class="text-xs font-medium text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-500/10 rounded-lg px-3 py-1.5">{removeError}</div>
    {/if}
  </div>

  {#if running}
    <LogViewer path={'/api/logs/' + container} />
  {/if}
</DetailPanel>
