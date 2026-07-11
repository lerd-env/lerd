import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import PresetSuggestionBanner from './PresetSuggestionBanner.svelte';
import { presets, type Preset } from '$stores/presets';
import { dismissedSuggestions } from '$stores/presetSuggestions';
import type { Service } from '$stores/services';

vi.mock('$stores/presets', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/presets')>();
  return { ...actual, loadPresets: vi.fn() };
});

function svc(over: Partial<Service> & { name: string }): Service {
  return { status: 'active', site_count: 0, ...over };
}

const openSearchDashboards: Preset = {
  name: 'opensearch-dashboards',
  description: 'OpenSearch Dashboards web UI',
  installed: false,
  admin_for: ['opensearch']
};

beforeEach(() => {
  presets.set([openSearchDashboards]);
  dismissedSuggestions.set([]);
});

describe('PresetSuggestionBanner', () => {
  // The whole point of admin_for: a store preset the binary has never heard of
  // gets suggested on the page of the service it declares it administers.
  it('offers the admin UI on the page of the service it administers', () => {
    const { getByText } = render(PresetSuggestionBanner, {
      props: { svc: svc({ name: 'opensearch' }) }
    });
    expect(getByText(/opensearch-dashboards/)).toBeTruthy();
  });

  it('renders nothing on a service nothing administers', () => {
    const { container } = render(PresetSuggestionBanner, {
      props: { svc: svc({ name: 'mailpit' }) }
    });
    expect(container.textContent?.trim()).toBe('');
  });

  it('renders nothing once the admin UI is installed', () => {
    presets.set([{ ...openSearchDashboards, installed: true }]);
    const { container } = render(PresetSuggestionBanner, {
      props: { svc: svc({ name: 'opensearch' }) }
    });
    expect(container.textContent?.trim()).toBe('');
  });

  it('renders nothing once dismissed', () => {
    dismissedSuggestions.set(['opensearch-dashboards']);
    const { container } = render(PresetSuggestionBanner, {
      props: { svc: svc({ name: 'opensearch' }) }
    });
    expect(container.textContent?.trim()).toBe('');
  });

  // A versioned member resolves through its preset, which is what replaced the
  // old connection_url regex.
  it('offers phpmyadmin on a versioned mariadb service', () => {
    presets.set([
      { name: 'phpmyadmin', description: 'MySQL admin UI', installed: false, admin_for: ['mysql', 'mariadb'] }
    ]);
    const { getByText } = render(PresetSuggestionBanner, {
      props: { svc: svc({ name: 'mariadb-11-8', preset: 'mariadb' }) }
    });
    expect(getByText(/phpmyadmin/)).toBeTruthy();
  });
});
