// Run against a fresh, disposable Overmynd instance; this creates its admin.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.BROWSER_CHANNEL ? { channel: process.env.BROWSER_CHANNEL } : {}) });
  try {
    const context = await browser.newContext();
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
    await page.waitForFunction(() => document.getElementById('sidebarHealth').textContent === 'Online');
    await page.locator('[data-view="services"]').click();
    await page.locator('#authForm').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#authSubmit').textContent(), 'Create administrator');
    await page.locator('#authUsername').fill('admin');
    await page.locator('#authPassword').fill('correct horse battery staple');
    await page.locator('#authSubmit').click();
    await page.locator('#servicesView.active').waitFor();
    await page.locator('#addServiceButton').click();
    await page.locator('#serviceType').selectOption('radarr');
    await page.locator('#serviceName').fill('Test Radarr');
    await page.locator('#serviceBaseURL').fill('http://127.0.0.1:1');
    await page.locator('#serviceCredential').fill('test-secret');
    await page.locator('#serviceSaveButton').click();
    await page.locator('[data-service-action="edit"]').waitFor();
    await page.evaluate(() => refreshDashboard());
    await page.locator('[data-service-action="edit"]').click();
    assert.equal(await page.locator('#serviceBaseURL').inputValue(), 'http://127.0.0.1:1');
    assert.equal(await page.locator('#serviceCredential').inputValue(), '');
    await page.locator('#serviceCancelButton').click();
    for (const width of [320, 375, 768, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      await page.locator('[data-view="settings"]').click();
      assert(await page.locator('#logoutButton').isVisible());
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `horizontal overflow at ${width}`);
    }
    await page.locator('#logoutButton').click();
    await page.locator('#authForm').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#serviceCredential').inputValue(), '');
    assert.equal(await page.locator('#serviceManagementList').textContent().then(s => s.includes('127.0.0.1:1')), false);
    await page.locator('#authUsername').fill('admin');
    await page.locator('#authPassword').fill('wrong-password');
    await page.locator('#authSubmit').click();
    await page.waitForFunction(() => document.getElementById('authMessage').textContent.includes('invalid'));
    await page.locator('#authPassword').fill('correct horse battery staple');
    await page.locator('#authSubmit').click();
    await page.locator('#servicesView.active').waitFor();
    await context.clearCookies();
    await page.locator('[data-view="dashboard"]').click();
    await page.locator('[data-view="services"]').click();
    await page.locator('#authForm').waitFor({ state: 'visible' });
    await page.locator('[data-view="dashboard"]').click();
    await page.evaluate(() => refreshDashboard());
    assert.equal(await page.locator('#sidebarHealth').textContent(), 'Online');
    assert.deepEqual(errors, []);
    console.log('PASS: public dashboard, setup, service editing, logout/login, expiry, responsive layout');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
