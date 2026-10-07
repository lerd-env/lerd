import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ServicesDashboard from './ServicesDashboard.svelte';

describe('ServicesDashboard', () => {
  it('keeps the header out of the scrolling area', () => {
    const { container } = render(ServicesDashboard);
    const header = container.querySelector('.page-header')!;
    expect(header.closest('.overflow-y-auto')).toBeNull();
    expect(container.querySelector('.overflow-y-auto')).not.toBeNull();
  });
});
