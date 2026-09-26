import { writable, derived, get } from 'svelte/store';
import { services, serviceAction, serviceOpenable, type Service } from './services';
import { adminServiceFor } from './presetSuggestions';
import { entities } from './entities';
import { DEFAULT_DOCS_ROUTE, parseDocsHash } from './docs';

export interface DashboardRef {
  name: string;
  label?: string;
  dashboard: string;
  // The service's declared icon key. Absent on the synthetic docs and profiler
  // refs, which are named in the UI-only icon map instead.
  icon?: string;
  // extraPath is appended to dashboard for the iframe src, used to deep-link
  // a service overlay (e.g. mailpit's /view/{id} for a captured email).
  extraPath?: string;
}

// The currently-open dashboard, either a real service or the synthetic 'docs' ref.
export const dashboardOpen = writable<DashboardRef | null>(null);

// The documentation renders in place of the iframe, from the pages embedded in
// the binary; `dashboard` is only the lerd.sh twin the header links out to.
const DOCS_REF: DashboardRef = {
  name: 'docs',
  label: 'Documentation',
  dashboard: 'https://lerd.sh'
};

// PROFILER_REF is the synthetic entry for the SPX profiler. The UI is proxied
// same-origin under /_spx/ by lerd-ui so the overlay can drive the iframe
// (back, reload) directly. /_spx/ reaches the profiler.localhost nginx vhost,
// which routes to a PHP-FPM container where SPX serves its report UI.
const PROFILER_REF: DashboardRef = {
  name: 'profiler',
  label: 'Profiler',
  dashboard: '/_spx/?SPX_UI_URI=/'
};

function fallbackHash(): string {
  const h = location.hash.slice(1);
  for (const t of ['sites', 'services', 'system']) {
    if (h === t || h.startsWith(t + '/')) return t;
  }
  return 'sites';
}

export function openDashboard(svc: Service) {
  if (svc.dashboard_external && svc.dashboard) {
    window.open(svc.dashboard, '_blank', 'noopener,noreferrer');
    return;
  }
  if (!svc.dashboard) return;
  const cur = get(dashboardOpen);
  if (cur && cur.name === svc.name) {
    dashboardOpen.set(null);
    location.hash = fallbackHash();
    return;
  }
  dashboardOpen.set({ name: svc.name, label: svc.name, dashboard: svc.dashboard, icon: svc.icon });
  location.hash = 'service/' + svc.name;
}

// openServiceDashboard opens a dashboard, starting the service first when it is
// installed but stopped. Shared by the service page and the site overview card
// so an admin tool is reached the same way from either. A failed start opens
// nothing; the overlay would only embed a URL nothing is listening on.
export async function openServiceDashboard(svc: Service) {
  if (svc.status !== 'active' && !(await serviceAction(svc.name, 'start'))) return;
  openDashboard(get(services).find((s) => s.name === svc.name) || svc);
}

// openMailpitMessage opens the mailpit dashboard overlay with the iframe
// pointed at /view/<id> so a clicked email notification lands the user on
// the captured message instead of mailpit's inbox.
export function openMailpitMessage(id: string) {
  const mp = get(services).find((s) => s.name === 'mailpit');
  if (!mp?.dashboard) return;
  const safeId = encodeURIComponent(id);
  dashboardOpen.set({
    name: 'mailpit',
    label: 'Mailpit',
    dashboard: mp.dashboard,
    icon: mp.icon,
    extraPath: '/view/' + safeId
  });
  location.hash = 'service/mailpit/view/' + safeId;
}

// openEntityInDashboard opens the service's own dashboard on one entity, from
// the address the preset declares for it. The link is the upstream's to describe,
// so nothing here knows what a bucket or an index is called inside it.
export async function openEntityInDashboard(svc: Service, kind: string, name: string) {
  const link = entityLinkFor(svc, kind);
  if (!svc.dashboard || !link) return;
  if (svc.status !== 'active' && !(await serviceAction(svc.name, 'start'))) return;
  const current = get(services).find((s) => s.name === svc.name) || svc;
  dashboardOpen.set({
    name: current.name,
    label: current.name,
    dashboard: current.dashboard || svc.dashboard,
    icon: current.icon,
    extraPath: entityExtraPath(link, name)
  });
  // The entity rides in the route, so a reload or a re-hydrate opens it again
  // rather than snapping the frame back to the dashboard's front page.
  location.hash =
    'service/' + current.name + '/entity/' + encodeURIComponent(kind) + '/' + encodeURIComponent(name);
}

// entityExtraPath fills the address a preset declares for one entity. The link
// is the upstream's to describe, so nothing here knows what a bucket or an index
// is called inside it.
export function entityExtraPath(link: string, name: string): string {
  return link.replaceAll('{{name}}', encodeURIComponent(name));
}

// entityLinkFor reads the declared address from the kinds already loaded for
// this service.
function entityLinkFor(svc: Service, kind: string): string {
  return get(entities)[svc.name]?.find((k) => k.kind === kind)?.dashboard_link ?? '';
}

// DB_DEEP_LINK maps an admin tool to the URL suffix that opens a specific
// database inside it, keyed by the preset the admin service was installed from.
// Only tools with a stable single-database URL are listed; pgAdmin has none, so
// its databases open the tool at its root via the engine header button instead.
const DB_DEEP_LINK: Record<string, (db: string) => string> = {
  phpmyadmin: (db) => `?db=${encodeURIComponent(db)}`,
  adminer: (db) => `?db=${encodeURIComponent(db)}`,
  'mongo-express': (db) => `/db/${encodeURIComponent(db)}`
};

// ENGINE_LINK is for an admin tool that fronts more than one engine at once.
// Opened at its root it picks one for you, which is the wrong one as often as
// not, so the engine it was opened from travels in the URL. Only the container
// name is passed: which driver and credentials go with it is the tool's own
// business, not something the dashboard should carry a table for.
const ENGINE_LINK: Record<string, (engine: string, db: string) => string> = {
  adminer: (engine, db) =>
    `?lerd_server=lerd-${engine}` + (db ? `&db=${encodeURIComponent(db)}` : '')
};

function engineLinker(admin: Service): ((engine: string, db: string) => string) | undefined {
  return ENGINE_LINK[admin.preset || admin.name];
}

function dbDeepLinker(admin: Service): ((db: string) => string) | undefined {
  return DB_DEEP_LINK[admin.preset || admin.name];
}

// databaseAdminFor returns the installed admin tool for the named engine, or
// null when none is installed. Tools with a database URL (phpMyAdmin, Adminer,
// Mongo Express) open on the database; pgAdmin, which has no per-database URL,
// opens at its root.
export function databaseAdminFor(engineName: string): Service | null {
  const list = get(services);
  const engine = list.find((s) => s.name === engineName);
  if (!engine) return null;
  const admin = adminServiceFor(engine, list);
  return admin?.dashboard ? admin : null;
}

// openDatabaseAdmin opens the engine's admin tool, deep-linked to one database
// when the tool supports it, starting it first when stopped. The database is
// encoded into the route hash (service/<admin>/db/<name>) so a later re-hydrate
// keeps the deep-link instead of snapping the iframe back to the tool's root.
export async function openDatabaseAdmin(engineName: string, database: string) {
  const admin = databaseAdminFor(engineName);
  if (!admin) return;
  if (admin.status !== 'active' && !(await serviceAction(admin.name, 'start'))) return;
  if (engineLinker(admin)) {
    location.hash =
      `service/${admin.name}/on/${engineName}` +
      (database ? `/${encodeURIComponent(database)}` : '');
    return;
  }
  location.hash = dbDeepLinker(admin)
    ? `service/${admin.name}/db/${encodeURIComponent(database)}`
    : `service/${admin.name}`;
}

// openAdminForEngine opens an engine's admin tool scoped to that engine, for
// the header button on a database service's own page, where no one database is
// in play yet. Starts the tool first when it is stopped, like openDatabaseAdmin.
export async function openAdminForEngine(engineName: string) {
  const admin = databaseAdminFor(engineName);
  if (!admin) return;
  if (admin.status !== 'active' && !(await serviceAction(admin.name, 'start'))) return;
  location.hash = engineLinker(admin)
    ? `service/${admin.name}/on/${engineName}`
    : `service/${admin.name}`;
}

export function openDocs() {
  const cur = get(dashboardOpen);
  if (cur && cur.name === 'docs') {
    dashboardOpen.set(null);
    location.hash = fallbackHash();
    return;
  }
  dashboardOpen.set(DOCS_REF);
  location.hash = 'docs/' + DEFAULT_DOCS_ROUTE;
}

export function openProfiler() {
  const cur = get(dashboardOpen);
  if (cur && cur.name === 'profiler') {
    dashboardOpen.set(null);
    location.hash = fallbackHash();
    return;
  }
  dashboardOpen.set(PROFILER_REF);
  location.hash = 'profiler';
}

export function closeDashboard() {
  dashboardOpen.set(null);
  location.hash = fallbackHash();
}

// Services eligible for an iframe dashboard entry (running or asleep + has dashboard + not external-only).
export const dashboardServices = derived(services, ($s) =>
  $s.filter((x) => serviceOpenable(x) && x.dashboard && !x.dashboard_external)
);

function refFromHash(): DashboardRef | null {
  const h = location.hash.slice(1);
  if (parseDocsHash(h)) return DOCS_REF;
  if (h === 'profiler') return PROFILER_REF;
  if (h.startsWith('service/')) {
    const rest = h.slice('service/'.length);
    // service/mailpit/view/<id> deep-links into a specific captured email.
    const mpDeep = rest.match(/^mailpit\/view\/(.+)$/);
    if (mpDeep) {
      const mp = get(services).find((x) => x.name === 'mailpit');
      if (mp?.dashboard) {
        return {
          name: 'mailpit',
          label: 'Mailpit',
          dashboard: mp.dashboard,
          icon: mp.icon,
          extraPath: '/view/' + mpDeep[1]
        };
      }
    }
    // service/<name>/entity/<kind>/<entity> opens one entity inside the
    // service's own dashboard, at the address its preset declares.
    const entityDeep = rest.match(/^(.+?)\/entity\/([^/]+)\/(.+)$/);
    if (entityDeep) {
      const svc = get(services).find((x) => x.name === entityDeep[1]);
      const link = svc ? entityLinkFor(svc, decodeURIComponent(entityDeep[2])) : '';
      if (svc?.dashboard && link) {
        return {
          name: svc.name,
          label: svc.name,
          dashboard: svc.dashboard,
          icon: svc.icon,
          extraPath: entityExtraPath(link, decodeURIComponent(entityDeep[3]))
        };
      }
    }
    // service/<admin>/on/<engine>[/<database>] scopes a multi-engine admin tool
    // to the engine it was opened from.
    const onEngine = rest.match(/^(.+?)\/on\/([^/]+)(?:\/(.+))?$/);
    if (onEngine) {
      const admin = get(services).find((x) => x.name === onEngine[1]);
      const linker = admin ? engineLinker(admin) : undefined;
      if (admin?.dashboard && linker) {
        return {
          name: admin.name,
          label: admin.name,
          dashboard: admin.dashboard,
          icon: admin.icon,
          extraPath: linker(onEngine[2], onEngine[3] ? decodeURIComponent(onEngine[3]) : '')
        };
      }
    }
    // service/<admin>/db/<database> deep-links an admin tool to one database.
    const dbDeep = rest.match(/^(.+?)\/db\/(.+)$/);
    if (dbDeep) {
      const admin = get(services).find((x) => x.name === dbDeep[1]);
      const linker = admin ? dbDeepLinker(admin) : undefined;
      if (admin?.dashboard && linker) {
        return {
          name: admin.name,
          label: admin.name,
          dashboard: admin.dashboard,
          icon: admin.icon,
          extraPath: linker(decodeURIComponent(dbDeep[2]))
        };
      }
    }
    const svc = get(services).find((x) => x.name === rest);
    if (svc?.dashboard)
      return { name: svc.name, label: svc.name, dashboard: svc.dashboard, icon: svc.icon };
  }
  return null;
}

export function initDashboardRoute() {
  dashboardOpen.set(refFromHash());
  window.addEventListener('hashchange', () => {
    dashboardOpen.set(refFromHash());
  });
  // Re-hydrate when services load so a #service/<name> deep-link resolves.
  services.subscribe(() => {
    const h = location.hash.slice(1);
    if (h.startsWith('service/')) {
      dashboardOpen.set(refFromHash());
    }
  });
}
