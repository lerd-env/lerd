import { render, fireEvent } from '@testing-library/svelte';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import SiteSuggestedServiceCard from './SiteSuggestedServiceCard.svelte';

const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

const { loadSites } = vi.hoisted(() => ({ loadSites: vi.fn() }));
vi.mock('$stores/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/sites')>()),
  loadSites
}));

const { confirmDownload } = vi.hoisted(() => ({ confirmDownload: vi.fn() }));
vi.mock('$stores/downloadConfirm', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$stores/downloadConfirm')>()),
  confirmDownload
}));

function ok() {
  return new Response(JSON.stringify({ ok: true }), { headers: { 'Content-Type': 'application/json' } });
}

describe('SiteSuggestedServiceCard', () => {
  beforeEach(() => {
    apiFetch.mockReset();
    apiFetch.mockResolvedValue(ok());
    confirmDownload.mockReset();
    confirmDownload.mockResolvedValue(true);
  });

  // Adding may install the service, so it asks before downloading its image,
  // and a decline leaves the site as it was.
  it('asks before downloading and adds nothing on a decline', async () => {
    confirmDownload.mockResolvedValue(false);
    const { getByLabelText } = render(SiteSuggestedServiceCard, { suggestion: { name: 'solr', reason: 'Search backend' }, domain: 'shop.test' });
    await fireEvent.click(getByLabelText(/add/i));
    await vi.waitFor(() =>
      expect(confirmDownload).toHaveBeenCalledWith(expect.any(String), { service: 'solr', action: 'add' })
    );
    expect(apiFetch).not.toHaveBeenCalled();
  });

  it('adds the service to the site it was suggested for', async () => {
    const { getByLabelText } = render(SiteSuggestedServiceCard, { suggestion: { name: 'solr', reason: 'Search backend', package: 'drupal/search_api_solr' }, domain: 'shop.test' });
    await fireEvent.click(getByLabelText(/add/i));
    expect(apiFetch).toHaveBeenCalledWith('/api/sites/shop.test/service:add?name=solr', { method: 'POST' });
    await vi.waitFor(() => expect(loadSites).toHaveBeenCalled());
  });

  it('says why the service is suggested', () => {
    const { container } = render(SiteSuggestedServiceCard, {
      suggestion: { name: 'solr', reason: 'Search backend', package: 'drupal/search_api_solr' },
      domain: 'shop.test'
    });
    expect(container.textContent).toContain('Search backend');
  });

  it('dismisses the suggestion for that site', async () => {
    const { getByLabelText } = render(SiteSuggestedServiceCard, { suggestion: { name: 'solr', reason: 'Search backend', package: 'drupal/search_api_solr' }, domain: 'shop.test' });
    await fireEvent.click(getByLabelText(/suggest/i));
    expect(apiFetch).toHaveBeenCalledWith('/api/sites/shop.test/service:dismiss?name=solr', { method: 'POST' });
  });

  // A suggestion must not read as a service the site already has: the card is
  // dashed and its label faded until hovered, while its actions stay clickable.
  it('fades the suggestion but not its actions', () => {
    const { container, getByTestId, getByLabelText } = render(SiteSuggestedServiceCard, {
      suggestion: { name: 'solr', reason: 'Search backend' },
      domain: 'shop.test'
    });
    expect(container.firstElementChild?.className).toContain('border-dashed');
    const identity = getByTestId('suggested-identity').className;
    expect(identity).toContain('opacity-60');
    expect(identity).toContain('group-hover:opacity-100');
    expect(identity).toContain('grayscale');
    expect(identity).toContain('group-hover:grayscale-0');
    expect(getByLabelText('Add Solr to this site').className).not.toContain('opacity-60');
  });
});

