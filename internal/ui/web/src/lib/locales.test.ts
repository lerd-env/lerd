import { describe, it, expect } from 'vitest';
import en from '../../messages/en.json';
import es from '../../messages/es.json';
import pt from '../../messages/pt.json';
import fr from '../../messages/fr.json';
import de from '../../messages/de.json';
import id from '../../messages/id.json';
import nl from '../../messages/nl.json';
import { LOCALES, LOCALE_LABELS, LOCALE_CODES } from '../stores/locale';

const locales: Record<string, Record<string, unknown>> = { en, es, pt, fr, de, id, nl };
const baseKeys = Object.keys(en).sort();

describe('UI locale message files', () => {
  for (const [code, msgs] of Object.entries(locales)) {
    if (code === 'en') continue;
    it(`${code}.json has the same keys as en.json (no drift)`, () => {
      const keys = Object.keys(msgs).sort();
      const missing = baseKeys.filter((k) => !keys.includes(k));
      const extra = keys.filter((k) => !baseKeys.includes(k));
      expect({ code, missing, extra }).toEqual({ code, missing: [], extra: [] });
    });

    it(`${code}.json declares a non-empty meta_language and meta_code`, () => {
      expect(typeof msgs.meta_language).toBe('string');
      expect(typeof msgs.meta_code).toBe('string');
      expect((msgs.meta_language as string).length).toBeGreaterThan(0);
      expect((msgs.meta_code as string).length).toBeGreaterThan(0);
    });
  }

  it('the locale store covers every compiled locale with a label and code', () => {
    for (const l of LOCALES) {
      expect(LOCALE_LABELS[l]).toBeTruthy();
      expect(LOCALE_CODES[l]).toBeTruthy();
      expect(locales).toHaveProperty(l);
    }
    expect(LOCALES).toContain('de');
    expect(LOCALES).toContain('id');
    expect(LOCALES).toContain('nl');
  });
});
