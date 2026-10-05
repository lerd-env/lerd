import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import WizardQuestions from './WizardQuestions.svelte';

const questions = {
  dir: '/home/u/acme',
  kind: 'php',
  kind_choice: false,
  node_managed: false,
  https_available: false,
  secured: false,
  service_options: ['mailpit', 'redis', 'mercure'],
  services: ['redis'],
  service_suggestions: [
    { name: 'redis', reason: 'Redis server for predis to connect to', package: 'predis/predis' },
    { name: 'mercure', reason: 'Realtime updates pushed over server-sent events' }
  ],
  frankenphp_offered: false,
  frankenphp: false,
  frankenphp_worker: false
};

const answers = { kind: 'php', secured: false, services: ['redis'], workers: [] };

describe('WizardQuestions', () => {
  // A suggested service says why it is ticked, naming the package that asked
  // for it the way the site's Overview card does; the rest stay bare.
  it('names the package behind a suggested service', () => {
    const { getByText, container } = render(WizardQuestions, { questions, answers, onchange: () => {} });

    expect(getByText('predis/predis')).toBeTruthy();
    expect(getByText('Realtime updates pushed over server-sent events')).toBeTruthy();
    const mailpit = getByText('mailpit').closest('label');
    expect(mailpit?.textContent?.trim()).toBe('mailpit');
    expect(container.querySelectorAll('input[type=checkbox]:checked').length).toBe(1);
  });
});
