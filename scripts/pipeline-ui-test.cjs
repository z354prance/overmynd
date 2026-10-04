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
    assert(await card('download').evaluate(el => el.classList.contains('media-movie')));
    assert(await card('process').evaluate(el => el.classList.contains('media-manual')));
    assert.equal(await page.evaluate(() => mediaCardType({kind:'episode'})), 'show');
    assert.equal(await page.evaluate(() => mediaCardType({kind:'series'})), 'show');
    assert.equal(await card('download').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'25');
    assert.equal(await card('process').locator('[role="progressbar"]').getAttribute('aria-valuenow'),'62');
    assert.equal(await card('unknown').locator('[role="progressbar"]').getAttribute('aria-valuenow'),null);
    assert.equal(await card('unknown').locator('img').count(),1);
    assert(!(await card('unknown').textContent()).includes('Import Blocked'));
    assert((await card('unknown').textContent()).includes('awaiting the next step'));
    const sizes=await page.locator('#pipelineList .pipeline-card').evaluateAll(cards=>cards.map(c=>({width:c.getBoundingClientRect().width,height:c.getBoundingClientRect().height})));
    assert(sizes.every(s=>s.width===sizes[0].width && s.height===112 && s.width>180));
    assert(await page.locator('#pipelineList .pipeline-card').evaluateAll(cards=>cards.every(card=>{
      const heading=card.querySelector('.pipeline-card-heading').getBoundingClientRect();
      const stage=card.querySelector('.pipeline-stage').getBoundingClientRect();
      const detail=card.querySelector('.pipeline-detail').getBoundingClientRect();
      const bar=card.querySelector('.pipeline-progress').getBoundingClientRect();
      return stage.bottom<=heading.bottom && stage.bottom<=detail.top && detail.bottom<=bar.top && bar.bottom<=card.getBoundingClientRect().bottom;
    })));
    assert(!(await page.locator('#pipelineList').textContent()).includes('\uFFFD'));
    assert.equal(await page.evaluate(()=>cleanMediaTitle({title:'Ella.Enchanted.2004.MULTi.1080p.WEB.mkv'})), 'Ella Enchanted');
    assert.equal(await page.evaluate(()=>cleanMediaTitle({title:'Taskmaster.S22E05.Youre.Nice.1080p.WEB.mkv'})), 'Taskmaster - S22E05');
    await card('process').locator('summary').click();
    assert(await page.locator('#cardDetailsDialog').isVisible());
    assert(!(await page.locator('#cardDetailsDialog').evaluate(el=>el.matches(':modal'))));
    assert((await page.locator('#cardDetailsContent').textContent()).includes('Tdarr'));
    await page.keyboard.press('Escape');
    assert(!(await page.locator('#cardDetailsDialog').isVisible()));

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
    assert.equal(await page.locator('#seriesOverviewList > article').first().getAttribute('data-lifecycle-id'),'series');
    assert.equal(await page.locator('#seriesOverviewList .series-progress-card').count(),1);
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#seriesOverviewList > article').first().getAttribute('data-lifecycle-id'),'series');
    assert.equal(await card('series').locator('[role=progressbar]').getAttribute('aria-valuenow'),'25');
    assert((await card('series').textContent()).includes('6 / 24 imported'));
    seasons=[{id:'s1',show_id:'showA',show_title:'24',season_number:1,total:24,imported:12},{id:'s2',show_id:'showA',show_title:'24',season_number:2,total:12,imported:6},{id:'s3',show_id:'showB',show_title:'24',season_number:1,total:10,imported:1}];
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#seriesOverviewList .series-progress-card').count(),2);
    assert(await page.locator('#seriesOverview').isVisible());
    assert.equal(await page.locator('#pipelineList .series-progress-card').count(),0);
    await card('showA').locator('summary').click();
    await page.evaluate(()=>refreshDashboard());
    assert(await page.locator('#cardDetailsDialog').isVisible());
    assert.equal(await page.locator('#cardDetailsContent .show-season-list > div').count(),2);
    await page.locator('#closeCardDetails').click();

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
    assert.equal(await page.locator('#seriesOverviewList > article').first().getAttribute('data-lifecycle-id'),'sonarr-season');
    assert.equal(await page.locator('#seriesOverviewList > article').count(),1);
    assert.equal(await page.locator('#pipelineList > article').count(),3);
    downloads.find(d=>d.id==='pending-2').status='downloading';
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await card('pending-2').count(),1);
    assert.equal(await card('pending-3').count(),0);
    seasons=[{id:'s1',show_id:'showA',show_title:'24',season_number:1,total:24,imported:12},{id:'s2',show_id:'showA',show_title:'24',season_number:2,total:12,imported:6},{id:'s3',show_id:'showB',show_title:'24',season_number:1,total:10,imported:1}];
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#seriesOverviewList .series-progress-card').count(),2);
    assert(await page.locator('#seriesOverview').isVisible());
    assert.equal(await page.locator('#pipelineList .series-progress-card').count(),0);
    await card('showA').locator('summary').click();
    await page.evaluate(()=>refreshDashboard());
    assert(await page.locator('#cardDetailsDialog').isVisible());
    assert.equal(await page.locator('#cardDetailsContent .show-season-list > div').count(),2);
    await page.locator('#closeCardDetails').click();

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
    jobs=[{id:'ep1',title:'Show S01E01',source:'tdarr',source_service_id:3,state:'processing',progress:20},{id:'ep2',title:'Show S01E02',source:'tdarr',source_service_id:3,state:'processing',progress:80}];
    downloads=[{id:'dl1',title:'Show S01E03',source:'nzbget',source_service_id:4,status:'downloading',size:100,size_left:70},{id:'dl2',title:'Show S01E04',source:'nzbget',source_service_id:4,status:'downloading',size:100,size_left:10}];
    items.push({id:'grouped',title:'Show',stage:'processing',references:[...jobs.map(j=>reference('processing',j.source,j.source_service_id,j.id)),...downloads.map(d=>reference('download',d.source,d.source_service_id,d.id))]});
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('#pipelineList .pipeline-card').count(),4);
    const values=await page.locator('#pipelineList [role=progressbar]').evaluateAll(nodes=>nodes.map(n=>Number(n.getAttribute('aria-valuenow'))).sort((a,b)=>a-b));
    assert.deepEqual(values,[20,30,80,90]);
    assert(!(await page.locator('#pipelineList').textContent()).includes('Average across'));
    downloads.push({id:'stale',title:'Show S01E01',source:'sonarr',source_service_id:5,status:'downloading',size:100,size_left:20});
    // ARR-only fixture: both the stale mirror and a different release share a lifecycle.
    downloads.push({id:'upgrade',title:'Show S01E01.1080p.OtherRelease',source:'sonarr',source_service_id:5,status:'downloading',size:100,size_left:20});
    items[0].references = [...jobs.map(j=>reference('processing',j.source,j.source_service_id,j.id)),...downloads.filter(d=>d.source==='sonarr').map(d=>reference('download',d.source,d.source_service_id,d.id))];
    jobs[0].title='Show S01E01.mkv';
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('[data-lifecycle-id="task:download:sonarr:5:stale"]').count(),0);
    assert.equal(await page.locator('[data-lifecycle-id="task:download:sonarr:5:upgrade"]').count(),1);
    jobs[0].state='queued';
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('[data-lifecycle-id="task:download:sonarr:5:stale"]').count(),1);
    jobs[0].state='problem';
    await page.evaluate(()=>refreshDashboard());
    assert.equal(await page.locator('[data-lifecycle-id="task:download:sonarr:5:stale"]').count(),1);

    items.length=0;
    await page.evaluate(()=>refreshDashboard());
    recent = Array.from({length:6},(_,i)=>({id:`ready-${i}`,title:i % 2 ? 'A much longer show title that needs two lines' : `Finished ${i}`,kind:i % 2 ? 'series' : 'movie',episodes:i % 2 ? ['Episode one', 'Episode two', 'Episode three'] : [],added_at:'2026-09-30T12:00:00Z'}));
    await page.evaluate(()=>refreshRecentlyAdded());
    assert.equal(await page.locator('#recentlyAddedList .recent-poster-card').count(),6);
    assert.equal(await page.locator('#recentlyAddedList .playback-art').count(),6);
    assert.equal(await page.locator('#recentlyAddedList .pipeline-steps').count(),0);
    for (const width of [320,768,1440,1800]) {
      await page.setViewportSize({width,height:1000});
      assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
      const boxes=await page.locator('#recentlyAddedList .recent-poster-card').evaluateAll(cards=>cards.map(c=>({w:c.offsetWidth,h:c.offsetHeight,top:c.offsetTop})));
      assert(boxes.every(b=>b.w===boxes[0].w && b.h===(width<=540?78:62)));
      const panelWidth=await page.locator('[aria-labelledby="recentlyAddedHeading"]').evaluate(el=>el.clientWidth);
      assert.equal(boxes.filter(b=>b.top===boxes[0].top).length,1);
      await page.locator('#recentlyAddedList details summary').nth(1).click();
      assert(await page.locator('#cardDetailsDialog').isVisible());
      assert.equal(await page.locator('#cardDetailsContent li').count(),3);
      await page.locator('#closeCardDetails').click();
      assert.equal(await page.locator('#recentlyAddedList .recent-poster-card').nth(1).evaluate(el=>el.offsetHeight),width<=540?78:62);
      assert(await page.locator('#recentlyAddedList').evaluate(el=>getComputedStyle(el).overflowY==='visible'));
      assert(await page.locator('#recentlyAddedList').evaluate(el=>el.scrollHeight<=el.clientHeight));
      const panelHeights = await page.locator('.playback-panel, [aria-labelledby="recentlyAddedHeading"]').evaluateAll(els=>els.map(el=>el.getBoundingClientRect().height));
      assert.equal(panelHeights[0],panelHeights[1]);
      await page.evaluate(()=>{state.playback=Array.from({length:14},(_,i)=>({id:`layout-${i}`,media_title:'Playback layout test',state:'playing'}));renderPlayback();});
      assert(await page.locator('#playbackList').evaluate(el=>getComputedStyle(el).overflowY==='auto' && el.scrollHeight>el.clientHeight));
      assert.equal(await page.locator('.playback-panel').evaluate(el=>el.getBoundingClientRect().height),panelHeights[0]);
      if(width>1000) {
        const bounds = await page.locator('.playback-panel, [aria-labelledby="recentlyAddedHeading"]').evaluateAll(els=>els.map(el=>el.getBoundingClientRect().bottom));
        assert.equal(bounds[0],bounds[1]);
      }

    }
    assert.deepEqual(errors,[]);
    console.log('PASS: lifecycle cards, measured/unknown progress, stage changes, source isolation, escaping, responsive layout');
  } finally { await browser.close(); }
})().catch(error => {console.error(error); process.exitCode=1;});
