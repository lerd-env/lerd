import { writable, derived, get } from 'svelte/store';
import { services, serviceAction, type Service } from './services';
import { adminServiceFor } from './presetSuggestions';

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

const DOCS_REF: DashboardRef = {
  name: 'docs',
  label: 'Documentation',
  dashboard: 'https://lerd.sh/getting-started/requirements'
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

// DB_DEEP_LINK maps an admin tool to the URL suffix that opens a specific
// database inside it, keyed by the preset the admin service was installed from.
// Only tools with a stable single-database URL are listed; pgAdmin has none, so
// its databases open the tool at its root via the engine header button instead.
const DB_DEEP_LINK: Record<string, (db: string) => string> = {
  phpmyadmin: (db) => `?db=${encodeURIComponent(db)}`,
  adminer: (db) => `?db=${encodeURIComponent(db)}`,
  'mongo-express': (db) => `/db/${encodeURIComponent(db)}`
};

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
  location.hash = dbDeepLinker(admin)
    ? `service/${admin.name}/db/${encodeURIComponent(database)}`
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
  location.hash = 'docs';
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

// Services eligible for an iframe dashboard entry (active + has dashboard + not external-only).
export const dashboardServices = derived(services, ($s) =>
  $s.filter((x) => x.status === 'active' && x.dashboard && !x.dashboard_external)
);

function refFromHash(): DashboardRef | null {
  const h = location.hash.slice(1);
  if (h === 'docs') return DOCS_REF;
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
    if (h.startsWith('service/') || h === 'docs') {
      dashboardOpen.set(refFromHash());
    }
  });
}
