import { writable } from 'svelte/store';
import {
  getLocale,
  setLocale as paraglideSetLocale,
  locales as paraglideLocales,
  localStorageKey
} from '../paraglide/runtime.js';

export type Locale = (typeof paraglideLocales)[number];

export const LOCALES: readonly Locale[] = paraglideLocales;

export const LOCALE_LABELS: Record<Locale, string> = {
  en: 'English',
  es: 'Español',
  pt: 'Português',
  fr: 'Français',
  de: 'Deutsch',
  id: 'Bahasa Indonesia',
  nl: 'Nederlands',
  tr: 'Türkçe',
  zh: '简体中文',
  ja: '日本語',
  ro: 'Română',
  it: 'Italiano',
  pl: 'Polski',
  vi: 'Tiếng Việt'
};

export const LOCALE_CODES: Record<Locale, string> = {
  en: 'EN',
  es: 'ES',
  pt: 'PT',
  fr: 'FR',
  de: 'DE',
  id: 'ID',
  nl: 'NL',
  tr: 'TR',
  zh: 'ZH',
  ja: 'JA',
  ro: 'RO',
  it: 'IT',
  pl: 'PL',
  vi: 'VI'
};

// Reactive store mirroring Paraglide's active locale. Paraglide stores the
// choice in localStorage via its configured strategy (localStorage →
// preferredLanguage → baseLocale), so this wrapper just surfaces it to Svelte
// for reactive re-renders when the user picks a new language.
export const locale = writable<Locale>(getLocale() as Locale);

// syncDashboardLocale hands a language picked here to the /_svc/ proxy, which
// states it to embedded dashboards in place of the browser's own. With no pick
// the cookie goes, and they follow the browser just as lerd does.
export function syncDashboardLocale() {
  const chosen = localStorage.getItem(localStorageKey);
  document.cookie = chosen
    ? `lerd_locale=${encodeURIComponent(chosen)}; path=/; SameSite=Strict`
    : 'lerd_locale=; path=/; max-age=0';
}

export function changeLocale(next: Locale) {
  // `reload: false` would ask Paraglide to swap without a page reload, but
  // messages are module-scoped functions compiled per-locale so everything
  // mounted keeps the previous value until the component re-evaluates. A full
  // reload is the safe path; the whole app boots in < 200 ms.
  paraglideSetLocale(next, { reload: true });
  locale.set(next);
  syncDashboardLocale();
}
