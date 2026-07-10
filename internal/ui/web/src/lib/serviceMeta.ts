import { derived } from 'svelte/store';
import { presets } from '$stores/presets';
import { services } from '$stores/services';

export interface ServiceMeta {
  category?: string;
  icon?: string;
}

// Name to declared metadata, so a component holding only a service name (a
// site's service chip) still draws the right icon. Installed services win over
// presets: a versioned member like "mariadb-11-8" exists only in the service
// list, already resolved through its preset server-side.
export const serviceMeta = derived([presets, services], ([$presets, $services]) => {
  const meta = new Map<string, ServiceMeta>();
  for (const p of $presets) {
    if (p.category || p.icon) meta.set(p.name, { category: p.category, icon: p.icon });
  }
  for (const s of $services) {
    if (s.category || s.icon) meta.set(s.name, { category: s.category, icon: s.icon });
  }
  return meta;
});
