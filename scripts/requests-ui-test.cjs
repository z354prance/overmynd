const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
  try {
    const page = await browser.newPage();
    const errors=[];page.on('pageerror',e=>errors.push(e.message));
    const submissions=[];
    let enabled=true;
    let deny=false;
    let savedSettings=null;
    await page.route('**/api/v1/**',route=>{
      const path=new URL(route.request().url()).pathname;
      if(path==='/api/v1/request-settings' && route.request().method()==='PUT') {
        savedSettings=route.request().postDataJSON();
        return route.fulfill({json:savedSettings});
      }
      if(path==='/api/v1/playback/poster'){
        if(new URL(route.request().url()).searchParams.has('broken')) return route.fulfill({status:404,body:''});
        return route.fulfill({contentType:'image/png',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jxQAAAABJRU5ErkJggg==','base64')});
      }
      if(path==='/api/v1/media-request'){
        submissions.push(route.request().postDataJSON());
        assert.equal(route.request().headers()['x-overmynd-request'],'1');
        return route.fulfill(deny?{status:502,json:{error:'Seerr denied access for the configured request user'}}:{status:201,json:{id:123,status:1}});
      }
      const payloads={
        '/api/v1/auth/status':{authenticated:false,setup_required:false},
        '/api/v1/services':[{id:4,type:'seerr',name:'Seerr',enabled:true}],
        '/api/v1/request-settings':{enabled:false,service_id:0,user_id:0},
        '/api/v1/media-request/status':{enabled},
        '/api/v1/media-request/search':{results:[
          {id:1,mediaType:'movie',title:'Arrival',releaseDate:'2016-01-01',overview:'A movie to request.'},
          {id:2,mediaType:'tv',name:'Planet Earth',overview:'A series to request.'},
          {id:3,mediaType:'movie',title:'Already available',mediaInfo:{status:5}},
        ]},
        '/api/v1/activity':{lifecycles:[]},'/api/v1/downloads':{downloads:[]},
        '/api/v1/missing':{items:[]},'/api/v1/processing':{jobs:[]},'/api/v1/public/services':[],
        '/api/v1/playback':{sessions:[
          {id:'one',media_title:'Arrival',state:'playing',duration_ms:100,progress_ms:50,poster_url:'/api/v1/playback/poster?test=1'},
          {id:'two',media_title:'No artwork',poster_url:'/api/v1/playback/poster?broken=1'},
        ]},
      };
      return route.fulfill({json:payloads[path]||[]});
    });
    await page.goto(process.env.OVERMYND_TEST_URL||'http://127.0.0.1:18080');
    await page.waitForFunction(()=>!document.getElementById('mediaSearchButton').disabled);
    assert.equal(await page.locator('#navigationDrawer').count(),0);
    assert.equal(await page.locator('#requestSettingsPanel').isVisible(),false);
    await page.locator('#openMediaRequest').click();
    await page.mouse.click(2,2);
    assert(!(await page.locator('#searchResultsPanel').isVisible()));
    await page.locator('#openMediaRequest').click();
    await page.locator('#mediaSearchQuery').fill('Arrival');
    await page.locator('#mediaSearchButton').click();
    await page.waitForFunction(()=>document.querySelectorAll('.request-result').length===3);
    assert(await page.locator('#searchResultsPanel').isVisible());
    await page.locator('#closeSearchResults').click();
    assert(!(await page.locator('#searchResultsPanel').isVisible()));
    await page.locator('#openMediaRequest').click();
    assert(await page.locator('#searchResultsPanel').isVisible());
    assert.deepEqual(await page.locator('#dashboardView .dashboard-grid > .panel').evaluateAll(nodes=>nodes.slice(0,2).map(n=>n.querySelector('h2').textContent)),['Now Playing','Recently Added']);

    assert(await page.locator('[data-request-index="2"]').isDisabled());
    await page.locator('[data-request-index="0"]').click();
    assert.equal(submissions.length,0);
    await page.locator('#requestConfirm').click();
    await page.waitForFunction(()=>document.getElementById('mediaRequestMessage').textContent.includes('Waiting for approval'));
    assert.deepEqual(submissions[0],{media_id:1,media_type:'movie',all_seasons:false});
    assert(await page.locator('[data-request-index="0"]').isDisabled());
    await page.locator('[data-request-index="1"]').click();
    await page.locator('#requestConfirm').click();
    assert.equal(submissions.length,1);
    await page.locator('#requestAllSeasons').check();
    deny=true;
    await page.locator('#requestConfirm').click();
    await page.waitForFunction(()=>document.getElementById('requestConfirmMessage').textContent.includes('denied'));
    assert.equal(submissions[1].all_seasons,true);
    await page.locator('#requestCancel').click();
    await page.locator('#closeSearchResults').click();
    assert.equal(await page.locator('#dashboardView .playback-panel').count(),1);
    await page.waitForFunction(()=>document.querySelectorAll('#playbackList .now-playing-card').length===2);
    assert(await page.locator('#playbackList .now-playing-card').first().evaluate(card=>{const art=card.querySelector('.playback-art').getBoundingClientRect();const info=card.querySelector('.now-playing-details').getBoundingClientRect();const bar=card.querySelector('.progress-track').getBoundingClientRect();return info.left>=art.right && bar.top>=info.bottom;}));
    const detailTrigger = page.locator('#playbackList details summary').first();
    await detailTrigger.click();
    assert(await page.locator('#cardDetailsDialog').isVisible());
    assert.equal(await page.locator('#playbackList details').first().evaluate(el=>el.open),false);
    assert.equal(await page.locator('#playbackList .now-playing-card').first().evaluate(el=>el.offsetHeight),94);
    assert(await page.locator('#cardDetailsContent .playback-facts').count());
    assert(!(await page.locator('#cardDetailsDialog').evaluate(el=>el.matches(':modal'))));
    assert(await page.locator('#cardDetailsDialog').evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0 && r.right<=innerWidth && r.top>=0 && r.bottom<=innerHeight;}));
    await page.locator('#closeCardDetails').click();
    assert(!(await page.locator('#cardDetailsDialog').isVisible()));
    await detailTrigger.click();
    await page.keyboard.press('Escape');
    assert(!(await page.locator('#cardDetailsDialog').isVisible()));
    await detailTrigger.click();
    await page.mouse.click(2,2);
    assert(!(await page.locator('#cardDetailsDialog').isVisible()));
    assert(await page.locator('#playbackList .playback-art').evaluateAll(els=>els.every(el=>getComputedStyle(el).display==='none')));
    assert(await page.locator('#playbackList .ascii-meter').count());
    for(const width of [320,375,768,1440]){
      await page.setViewportSize({width,height:1000});
        for(const view of ['services','settings']) assert(await page.locator(`[data-view="${view}"]`).isVisible());
      await page.evaluate(()=>setView('dashboard'));
      assert.equal(await page.locator('#navigationDrawer').count(),0);
      assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`overflow at ${width}`);
    }
    if(process.env.REQUESTS_SCREENSHOT) await page.screenshot({path:process.env.REQUESTS_SCREENSHOT,fullPage:true});
    if(!(await page.locator('#searchResultsPanel').evaluate(el=>el.open))) await page.locator('#openMediaRequest').click();
    await page.locator('#mediaSearchClear').click();
    assert.equal(await page.locator('#mediaSearchQuery').inputValue(),'');
    assert.equal(await page.locator('.request-result').count(),0);
    assert.equal(await page.locator('#mediaRequestMessage').textContent(),'');
    assert(await page.locator('#mediaSearchQuery').evaluate(e=>document.activeElement===e));
    let releaseSearch;
    const delayedSearch = new Promise(resolve=>releaseSearch=resolve);
    let searchStarted;
    const started = new Promise(resolve=>searchStarted=resolve);
    await page.route('**/api/v1/media-request/search?**',async route=>{
      searchStarted();
      await delayedSearch;
      await route.fulfill({json:{results:[{id:99,mediaType:'movie',title:'Late result'}]}});
    });
    await page.locator('#mediaSearchQuery').fill('Late');
    await page.locator('#mediaSearchButton').click();
    await started;
    if(!(await page.locator('#searchResultsPanel').evaluate(el=>el.open))) await page.locator('#openMediaRequest').click();
    await page.locator('#mediaSearchClear').click();
    const responseFinished=page.waitForResponse(r=>r.url().includes('/media-request/search?'));
    releaseSearch();
    await responseFinished;
    await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
    assert.equal(await page.locator('.request-result').count(),0);
    assert.equal(await page.locator('#mediaRequestMessage').textContent(),'');
    assert.equal(await page.locator('#mediaSearchButton').isDisabled(),false);
    enabled=false;
    await page.evaluate(()=>refreshRequestStatus());
    assert(await page.locator('#mediaSearchButton').isDisabled());
    await page.locator('#mediaSearchQuery').fill('Clear while disabled');
    if(!(await page.locator('#searchResultsPanel').evaluate(el=>el.open))) await page.locator('#openMediaRequest').click();
    await page.locator('#mediaSearchClear').click();
    assert((await page.locator('#mediaRequestMessage').textContent()).includes('not enabled'));
    assert(await page.locator('#mediaSearchButton').isDisabled());
    await page.evaluate(()=>applyAuth({authenticated:true,username:'admin'}));
    await page.locator('#closeSearchResults').click();
    await page.locator('[data-view="settings"]').click();
    await page.locator('#requestServiceID option[value="4"]').waitFor({state:'attached'});
    await page.locator('#requestsEnabled').check();
    await page.locator('#requestServiceID').selectOption('4');
    await page.locator('#requestUserID').fill('7');
    await page.locator('#requestSettingsSave').click();
    await page.waitForFunction(()=>document.getElementById('requestSettingsMessage').textContent.includes('Saved'));
    assert.deepEqual(savedSettings,{enabled:true,service_id:4,user_id:7});
    await page.locator('#openDonate').click();
    assert(await page.locator('#donatePopover').isVisible());
    assert.equal(await page.locator('#donatePopover a').first().getAttribute('href'),'https://paypal.me/z354prance');
    assert.equal(await page.locator('#donatePopover a').nth(1).getAttribute('href'),'https://cash.app/$PayLuffie');
    await page.keyboard.press('Escape');
    assert(!(await page.locator('#donatePopover').isVisible()));
    assert.deepEqual(errors,[]);
    console.log('PASS: drawer, public Seerr search/request, approval messaging, TV confirmation, artwork/fallback, mobile layout');
  } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
