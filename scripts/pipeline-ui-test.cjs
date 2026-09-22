const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.BROWSER_CHANNEL ? { channel: process.env.BROWSER_CHANNEL } : {}) });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const reference = (record_type, source, source_service_id, record_id) => ({ record_type, source, source_service_id, record_id });
    const items = [
      { id: 'new', title: 'A new request', stage: 'requested', references: [] },
      { id: 'download', title: 'Arrival', year: 2016, kind: 'movie', stage: 'downloading', references: [reference('download','qbittorrent',1,'same-id')] },
      { id: 'process', title: 'Planet Earth', stage: 'processing', references: [reference('processing','tdarr',3,'job')] },
      { id: 'unknown', title: '<img src=x onerror=alert(1)>', stage: 'importing', problems: ['import_blocked'], references: [] },
    ];
    let downloads = [
      { id:'same-id', source:'qbittorrent', source_service_id:1, title:'Arrival', size:1000, size_left:750, status:'downloading' },
      { id:'same-id', source:'qbittorrent', source_service_id:2, size:1000, size_left:0 },
    ];
    let jobs = [{ id:'job', source:'tdarr', source_service_id:3, state:'processing', progress:62, stage:'transcoding' }];
    await page.route('**/api/v1/**', route => {
      const path = new URL(route.request().url()).pathname;
      const payloads = {
        '/api/v1/activity': {lifecycles:items}, '/api/v1/downloads':{downloads},
        '/api/v1/processing':{jobs}, '/api/v1/missing':{items:[]},
        '/api/v1/playback':{sessions:[]}, '/api/v1/public/services':[],
        '/api/v1/auth/status':{authenticated:false,setup_required:false},
      };
      return route.fulfill({ json: payloads[path] || [] });
    });
    await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
    await page.waitForFunction(() => document.querySelectorAll('.pipeline-card').length === 4);
    const card = id => page.locator(`[data-lifecycle-id="${id}"]`);
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'25');
    assert.equal(await card('process').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'62');
    assert.equal(await card('unknown').locator('[role="progressbar"]').getAttribute('aria-valuenow'),null);
    assert.equal(await card('unknown').locator('img').count(),0);
    assert((await card('unknown').textContent()).includes('Import Blocked'));
    assert(!(await page.locator('#pipelineList').textContent()).includes('\uFFFD'));
    await page.evaluate(() => { window.originalCard = document.querySelector('[data-lifecycle-id="download"]'); });
    downloads[0].size_left = 200;
    await page.evaluate(() => refreshDashboard());
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'80');
    assert(await page.evaluate(() => window.originalCard === document.querySelector('[data-lifecycle-id="download"]')));
    items[1].stage = 'processing';
    items[1].references = [reference('processing','tdarr',3,'job')];
    jobs[0].progress = 0;
    await page.evaluate(() => refreshDashboard());
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'0');
    assert.equal(await card('download').locator('[aria-current="step"]').textContent(),'Process');
    delete jobs[0].progress;
    await page.evaluate(() => refreshDashboard());
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),null);
    for (const width of [320,375,768,1440]) {
      await page.setViewportSize({width,height:1000});
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `overflow at ${width}`);
    }
    if (process.env.PIPELINE_SCREENSHOT) await page.screenshot({path:process.env.PIPELINE_SCREENSHOT,fullPage:true});
    for(let i=0;i<12;i++) items.push({id:`extra-${i}`,title:`Queued item ${i}`,stage:'wanted',references:[]});
    await page.evaluate(() => refreshDashboard());
    assert.equal(await page.locator('.pipeline-card').count(),16);
    items.splice(0,items.length);
    await page.evaluate(() => refreshDashboard());
    assert.equal(await page.locator('.pipeline-card').count(),0);
    assert.deepEqual(errors,[]);
    console.log('PASS: lifecycle cards, measured/unknown progress, stage changes, source isolation, escaping, responsive layout');
  } finally { await browser.close(); }
})().catch(error => {console.error(error); process.exitCode=1;});
