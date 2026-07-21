import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import LensGroupLabel from './LensGroupLabel.svelte';
import { sites } from '$stores/sites';

describe('LensGroupLabel', () => {
  beforeEach(() => {
    sites.set([{ domain: 'admin-astrolov.test', name: 'admin-astrolov' }]);
  });

  it('links a known site name to that site Debug tab', () => {
    render(LensGroupLabel, { props: { label: { site: 'admin-astrolov', branch: '', text: 'GET /caffeine/drip' } } });
    const link = screen.getByText('admin-astrolov');
    expect(link.tagName.toLowerCase()).toBe('a');
    expect(link.getAttribute('href')).toBe('#sites/admin-astrolov.test/dumps');
  });

  it('keeps the branch inside the brackets and out of the link', () => {
    const { container } = render(LensGroupLabel, {
      props: { label: { site: 'admin-astrolov', branch: 'feature-x', text: 'GET /' } }
    });
    expect(screen.getByText('admin-astrolov').tagName.toLowerCase()).toBe('a');
    expect(container.textContent).toContain('[admin-astrolov@feature-x]');
    expect(container.querySelector('a')?.textContent).toBe('admin-astrolov');
  });

  it('leaves an unregistered site name as plain text', () => {
    const { container } = render(LensGroupLabel, {
      props: { label: { site: 'mystery', branch: '', text: 'GET /' } }
    });
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent).toContain('[mystery]');
  });

  it('renders a lone branch unlinked, as the per-site tab shows it', () => {
    const { container } = render(LensGroupLabel, {
      props: { label: { site: '', branch: 'feature-x', text: 'GET /' } }
    });
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent).toContain('[feature-x]');
    expect(container.textContent).toContain('GET /');
  });

  it('renders the label text alone when there is no site or branch', () => {
    const { container } = render(LensGroupLabel, {
      props: { label: { site: '', branch: '', text: 'cli (pid 7)' } }
    });
    expect(container.textContent?.trim()).toBe('cli (pid 7)');
  });
});
