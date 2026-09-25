import { render, fireEvent } from '@testing-library/svelte';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import SiteSuggestedServiceCard from './SiteSuggestedServiceCard.svelte';

const apiFetch = vi.fn();
vi.mock('$lib/api', async (orig) => ({
  ...(await orig<typeof import('$lib/api')>()),
  apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

function ok() {
  return new Response(JSON.stringify({ ok: true }), { headers: { 'Content-Type': 'application/json' } });
}

describe('SiteSuggestedServiceCard', () => {
  beforeEach(() => {
    apiFetch.mockReset();
    apiFetch.mockResolvedValue(ok());
  });

  it('adds the service to the site it was suggested for', async () => {
    const { getByLabelText } = render(SiteSuggestedServiceCard, { suggestion: { name: 'solr', reason: 'Search backend', package: 'drupal/search_api_solr' }, domain: 'shop.test' });
    await fireEvent.click(getByLabelText(/add/i));
    expect(apiFetch).toHaveBeenCalledWith('/api/sites/shop.test/service:add?name=solr', { method: 'POST' });
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
});
