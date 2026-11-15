// Click through the backend-free demo the way a visitor would: each rail
// section, each entry in its list column, then the tabs of each detail pane.
// Collects the demo's "no fixture for GET" warnings and page errors.
// Playwright is not a dependency of the UI; point PLAYWRIGHT at any install of it.
const pkg = (await import(process.env.PLAYWRIGHT || 'playwright')).default;
const { chromium } = pkg;
const BASE = process.argv[2] || 'http://127.0.0.1:4180/demo/';
const SECTIONS = ['Dashboard', 'Sites', 'Services', 'System', 'Profiler', 'Plugins', 'Notifications'];
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' });
const missing = new Map();
const errors = [];
let where = 'load';
page.on('console', (m) => {
  const hit = m.text().match(/\[demo\] no fixture for GET (\S+)/);
  if (hit && !missing.has(hit[1])) missing.set(hit[1], where);
});
page.on('pageerror', (e) => errors.push(`${where}: ${e.message}`));
await page.goto(BASE, { waitUntil: 'networkidle' });
const settle = (ms = 500) => page.waitForTimeout(ms);
const railX = 80;

// Short-labelled buttons in a screen region, as {x, y, label}.
const buttonsIn = (x0, x1, y0, y1) => page.$$eval('button, [role="tab"], a[href^="#"]', (els, b) => els
  .map((e) => { const r = e.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2, w: r.width, h: r.height, label: (e.getAttribute('aria-label') || e.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 40) }; })
  .filter((r) => r.w > 0 && r.h > 0 && r.x >= b[0] && r.x < b[1] && r.y >= b[2] && r.y < b[3] && r.label && !/delete|remove|uninstall|drop|stop|quit|reset|update all|install|restart|rebuild|clear/i.test(r.label)),
  [x0, x1, y0, y1]);

let clicks = 0;
for (const section of SECTIONS) {
  where = section;
  const rail = page.locator(`button[aria-label="${section}"], aside button:has-text("${section}")`).first();
  try { await rail.click({ timeout: 2000 }); } catch { continue; }
  await settle(800);
  const list = await buttonsIn(railX, 430, 60, 900);
  for (const item of list.slice(0, 30)) {
    where = `${section} › ${item.label}`;
    await page.mouse.click(item.x, item.y); clicks++;
    await settle(500);
    const tabs = await buttonsIn(430, 1440, 60, 330);
    for (const t of tabs.slice(0, 10)) {
      where = `${section} › ${item.label} › ${t.label}`;
      await page.mouse.click(t.x, t.y); clicks++;
      await settle(350);
      await page.keyboard.press('Escape');
    }
  }
}
console.log(`clicks: ${clicks}`);
console.log(`endpoints with no fixture: ${missing.size}`);
for (const [e, w] of [...missing].sort()) console.log(`  ${e}   (first at ${w})`);
console.log(`page errors: ${errors.length}`);
for (const e of errors.slice(0, 15)) console.log('  ' + e);
await browser.close();
