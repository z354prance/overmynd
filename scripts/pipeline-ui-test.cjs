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
    let recent = [];
    let seasons = [];
    let downloads = [
      { id:'same-id', source:'qbittorrent', source_service_id:1, title:'Arrival', size:1000, size_left:750, status:'downloading' },
      { id:'same-id', source:'qbittorrent', source_service_id:2, size:1000, size_left:0 },
    ];
    let jobs = [{ id:'job', source:'tdarr', source_service_id:3, state:'processing', progress:62, stage:'transcoding' }];
    await page.route('**/api/v1/**', route => {
      const path = new URL(route.request().url()).pathname;
      const payloads = {
        '/api/v1/recently-added': {items:recent,configured:true,errors:[]}, '/api/v1/activity': {lifecycles:items,seasons}, '/api/v1/downloads':{downloads},
        '/api/v1/processing':{jobs}, '/api/v1/missing':{items:[]},
        '/api/v1/playback':{sessions:[]}, '/api/v1/public/services':[],
        '/api/v1/auth/status':{authenticated:false,setup_required:false},
      };
      return route.fulfill({ json: payloads[path] || [] });
    });
    await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
    await page.waitForFunction(() => document.querySelectorAll('.pipeline-card').length === 3);
    assert.equal(await page.locator('[data-lifecycle-id="new"]').count(),0);
    const card = id => page.locator(`[data-lifecycle-id="${id}"]`);
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'25');
    assert.equal(await card('process').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'62');
    assert.equal(await card('unknown').locator('[role="progressbar"]').getAttribute('aria-valuenow'),null);
    assert.equal(await card('unknown').locator('img').count(),0);
    assert(!(await card('unknown').textContent()).includes('Import Blocked'));
    assert((await card('unknown').textContent()).includes('awaiting the next step'));
    const sizes=await page.locator('#pipelineList .pipeline-card').evaluateAll(cards=>cards.map(c=>({width:c.getBoundingClientRect().width,height:c.getBoundingClientRect().height})));
    assert(sizes.every(s=>s.width===sizes[0].width && s.height===252 && s.width<=260));
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
    assert.equal(await page.locator('.pipeline-card').count(),3);
    seasons=[{id:'series',title:'Zulu Series — Season 1',total:24,imported:6}];
    items.push({id:'series',title:'Zulu Series',kind:'series',stage:'downloading',references:[]});
    items.push({id:'episode',title:'A single episode',kind:'series',episode_number:2,stage:'importing',references:[]});
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList > article').first().getAttribute('data-lifecycle-id'),'series');
    assert.equal(await page.locator('#pipelineList .series-progress-card').count(),1);
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList > article').first().getAttribute('data-lifecycle-id'),'series');
    assert.equal(await card('series').locator('[role=progressbar]').getAttribute('aria-valuenow'),'25');
    assert((await card('series').textContent()).includes('6 / 24 imported'));
    seasons=[{id:'s1',show_id:'showA',show_title:'24',season_number:1,total:24,imported:12},{id:'s2',show_id:'showA',show_title:'24',season_number:2,total:12,imported:6},{id:'s3',show_id:'showB',show_title:'24',season_number:1,total:10,imported:1}];
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList .series-progress-card').count(),2);
    assert.equal(await card('showA').locator('.show-season-list > div').count(),2);
    assert.equal(await card('showA').locator('[role=progressbar]').getAttribute('aria-valuenow'),'50');
    seasons=[];
    items.splice(items.findIndex(item=>item.id==='series'),2);
    const extraStart=items.length;
    for(let i=0;i<99;i++) {
      downloads.push({id:`pending-${i}`,source:'nzbget',source_service_id:4,status:'queued',size:1000,size_left:1000});
      items.push({id:`pending-${i}`,title:`Pending episode ${i}`,kind:'episode',stage:'downloading',references:[reference('download','nzbget',4,`pending-${i}`)]});
    }
    seasons=[{id:'sonarr-season',title:'24 — Season 1',total:24,imported:12}];
    items.push({id:'aggregate',title:'24',kind:'episode',episode_number:12,stage:'downloading',references:[reference('download','nzbget',4,'pending-0'),reference('download','nzbget',4,'pending-1')]});
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList > article').first().getAttribute('data-lifecycle-id'),'sonarr-season');
    assert.equal(await page.locator('#pipelineList > article').count(),4);
    downloads.find(d=>d.id==='pending-2').status='downloading';
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await card('pending-2').count(),1);
    assert.equal(await card('pending-3').count(),0);
    seasons=[{id:'s1',show_id:'showA',show_title:'24',season_number:1,total:24,imported:12},{id:'s2',show_id:'showA',show_title:'24',season_number:2,total:12,imported:6},{id:'s3',show_id:'showB',show_title:'24',season_number:1,total:10,imported:1}];
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList .series-progress-card').count(),2);
    assert.equal(await card('showA').locator('.show-season-list > div').count(),2);
    assert.equal(await card('showA').locator('[role=progressbar]').getAttribute('aria-valuenow'),'50');
    seasons=[];
    items.splice(extraStart);
    items.splice(0,4);
    await page.evaluate(() => refreshDashboard());
    assert.equal(await page.locator('.pipeline-card').count(),0);
    assert.equal(await page.locator('#pipelineCount').textContent(),'0');
    items.splice(0,items.length);
    await page.evaluate(() => refreshDashboard());
    assert.equal(await page.locator('.pipeline-card').count(),0);
    assert.equal(await page.locator('#pipelineList .pipeline-steps li').filter({hasText:/^Import$/}).count(),0);
    recent = Array.from({length:6},(_,i)=>({id:`ready-${i}`,title:`Finished ${i}`,kind:'movie',added_at:'2026-09-30T12:00:00Z'}));
    await page.evaluate(()=>refreshRecentlyAdded());
    assert.equal(await page.locator('#recentlyAddedList .pipeline-card').count(),6);
    assert.equal(await page.locator('#recentlyAddedList [aria-valuenow="100"]').count(),6);
    assert.equal(await page.locator('#recentlyAddedList .pipeline-steps li').count(),24);
    assert.equal(await page.locator('#recentlyAddedList [aria-current="step"]').first().textContent(),'Ready');
    assert.equal(await page.locator('#pipelineList .pipeline-card').count(),0);
    for (const width of [320,768,1440]) {
      await page.setViewportSize({width,height:1000});
      assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    }
    assert.deepEqual(errors,[]);
    console.log('PASS: lifecycle cards, measured/unknown progress, stage changes, source isolation, escaping, responsive layout');
  } finally { await browser.close(); }
})().catch(error => {console.error(error); process.exitCode=1;});
