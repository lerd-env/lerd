import type { Preset } from '$stores/presets';
import { m } from '../paraglide/messages.js';

export type CategoryKey =
  | 'databases'
  | 'cache'
  | 'messaging'
  | 'search'
  | 'mail'
  | 'admin'
  | 'storage'
  | 'testing'
  | 'other';

// Display order for the discovery sections.
export const CATEGORY_ORDER: CategoryKey[] = [
  'databases',
  'cache',
  'messaging',
  'search',
  'mail',
  'admin',
  'storage',
  'testing',
  'other'
];

const BY_NAME: Record<string, CategoryKey> = {
  mysql: 'databases',
  mariadb: 'databases',
  postgres: 'databases',
  'postgres-pgvector': 'databases',
  'postgres-postgis': 'databases',
  mongo: 'databases',
  redis: 'cache',
  valkey: 'cache',
  memcached: 'cache',
  rabbitmq: 'messaging',
  beanstalkd: 'messaging',
  soketi: 'messaging',
  elasticsearch: 'search',
  opensearch: 'search',
  meilisearch: 'search',
  typesense: 'search',
  mailpit: 'mail',
  gotenberg: 'mail',
  phpmyadmin: 'admin',
  pgadmin: 'admin',
  'mongo-express': 'admin',
  redisinsight: 'admin',
  elasticvue: 'admin',
  'typesense-dashboard': 'admin',
  rustfs: 'storage',
  selenium: 'testing',
  'stripe-mock': 'testing'
};

// Display label per category. Record<CategoryKey, ...> makes a missing entry a
// compile error, so adding a category can't silently fall back to "Other".
export const CATEGORY_LABELS: Record<CategoryKey, () => string> = {
  databases: m.services_cat_databases,
  cache: m.services_cat_cache,
  messaging: m.services_cat_messaging,
  search: m.services_cat_search,
  mail: m.services_cat_mail,
  admin: m.services_cat_admin,
  storage: m.services_cat_storage,
  testing: m.services_cat_testing,
  other: m.services_cat_other
};

// Per-category icon tints, shared by every card that draws a service so one
// reads the same colour wherever it appears. Full static strings for Tailwind.
const ICON_TINT: Record<CategoryKey, string> = {
  databases: 'bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400',
  cache: 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-400',
  messaging: 'bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-400',
  search: 'bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-400',
  mail: 'bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-400',
  admin: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400',
  storage: 'bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-400',
  testing: 'bg-fuchsia-50 text-fuchsia-600 dark:bg-fuchsia-500/10 dark:text-fuchsia-400',
  other: 'bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400'
};

// tintFor resolves a service's icon tint, preferring an explicit category when
// the caller already knows it (a preset grouped under its section heading).
export function tintFor(name: string, category?: CategoryKey): string {
  return ICON_TINT[category ?? categoryOf(name)];
}

export function categoryOf(name: string): CategoryKey {
  if (BY_NAME[name]) return BY_NAME[name];
  // Fall back to the family prefix so versioned variants like "postgres-17"
  // land in the same bucket as their base preset.
  const fam = name.match(/^([a-z][a-z0-9]*)/);
  if (fam && BY_NAME[fam[1]]) return BY_NAME[fam[1]];
  return 'other';
}

export interface CategoryGroup {
  key: CategoryKey;
  presets: Preset[];
}

export function groupByCategory(presets: Preset[]): CategoryGroup[] {
  const buckets = new Map<CategoryKey, Preset[]>();
  for (const p of presets) {
    const k = categoryOf(p.name);
    const arr = buckets.get(k) || [];
    arr.push(p);
    buckets.set(k, arr);
  }
  return CATEGORY_ORDER.filter((k) => buckets.has(k)).map((k) => ({
    key: k,
    presets: buckets.get(k)!.slice().sort((a, b) => a.name.localeCompare(b.name))
  }));
}
