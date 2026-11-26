import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SiteControls from './SiteControls.svelte';
import type { Site } from '$stores/sites';

const site = (over: Partial<Site> = {}): Site =>
  ({ domain: 'app.test', name: 'app', custom_container: true, container_port: 80, ...over }) as Site;

describe('SiteControls build badge', () => {
  it('names a failed image build', () => {
    const { getByText } = render(SiteControls, { props: { site: site({ container_build_failed: true }) } });
    expect(getByText('build failed').getAttribute('title')).toContain('previous image');
  });

  it('stays out of the way otherwise', () => {
    const { queryByText } = render(SiteControls, { props: { site: site() } });
    expect(queryByText('build failed')).toBeNull();
  });
});
