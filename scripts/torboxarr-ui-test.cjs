const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
  const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const ref=(record_type,source,record_id)=>({record_type,source,record_id,source_service_id:1});
  let downloads=[{id:'local',source:'torboxarr',source_service_id:1,title:'Test.S01E01',size:104857600,size_left:78643200,status:'downloading',speed:524288,time_left:'0:01:00'},
   {id:'arr',source:'sonarr',source_service_id:1,title:'Test.S01E01',size:104857600,size_left:0,status:'completed'}];
  let jobs=[];
  let lifecycles=[{id:'test',title:'Test',kind:'episode',stage:'downloaded',references:[ref('download','torboxarr','local'),ref('download','sonarr','arr')]}];
  await page.route('**/api/v1/**',r=>{
   const payloads={'/api/v1/activity':{lifecycles,seasons:[]},'/api/v1/downloads':{downloads},'/api/v1/processing':{jobs},'/api/v1/missing':{items:[]},'/api/v1/playback':{sessions:[]},'/api/v1/recently-added':{items:[],errors:[]},'/api/v1/auth/status':{authenticated:false,setup_required:false}};
   return r.fulfill({json:payloads[new URL(r.request().url()).pathname]||[]});
  });
  await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
  const cards=page.locator('#pipelineList .pipeline-card');
  await cards.first().waitFor();
  assert.equal(await cards.count(),1);
  assert.equal(await cards.locator('[role=progressbar]').getAttribute('aria-valuenow'),'25');
  assert((await cards.textContent()).includes('TorBox to Unraid'));
  assert((await cards.textContent()).includes('512 KB/s'));
  assert((await cards.textContent()).includes('Transfer: Downloading'));
  for(const width of [320,768,1440]) {await page.setViewportSize({width,height:900});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));}
  // An ARR-only queue entry is not an active transfer merely because ARR says downloading.
  const pending={id:'pending',source:'sonarr',source_service_id:1,title:'Pending.S01E02',status:'downloading',size:1000,size_left:1000,time_left:'00:05:17.8228182'};
  downloads.push(pending);
  lifecycles.push({id:'pending',title:'Pending',kind:'episode',stage:'downloading',references:[ref('download','sonarr','pending')]});
  await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),1);
  pending.size_left=999;await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),2);
  pending.size_left=1000;pending.tracked_download_status='warning';await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),2);
  delete pending.tracked_download_status;
  pending.source='torboxarr';lifecycles[1].references=[ref('download','torboxarr','pending')];
  await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),2);
  pending.source='sonarr';lifecycles[1].references=[ref('download','sonarr','pending')];
  delete pending.size_left;await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),2);
  downloads.pop();lifecycles.pop();
  delete downloads[0].speed;delete downloads[0].size;delete downloads[0].size_left;
  await page.evaluate(()=>refreshDashboard());
  assert.equal(await cards.locator('[role=progressbar]').getAttribute('aria-valuenow'),null);
  assert(!(await cards.textContent()).includes('KB/s'));
  downloads[0].progress=42;await page.evaluate(()=>refreshDashboard());
  assert.equal(await cards.locator('[role=progressbar]').getAttribute('aria-valuenow'),'42');
  downloads[0].status='queued';await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),0);
  downloads[0].status='moving';downloads[0].size=100;downloads[0].size_left=0;
  await page.evaluate(()=>refreshDashboard());assert.equal(await cards.count(),1);
  assert((await cards.textContent()).includes('Transfer: Moving'));
  downloads=[];jobs=[{id:'job',source:'tdarr',source_service_id:1,title:'Test.S01E01',state:'processing',stage:'transcode',progress:15}];
  lifecycles=[{id:'test',title:'Test',kind:'episode',stage:'processing',references:[ref('processing','tdarr','job')]}];
  await page.evaluate(()=>refreshDashboard());
  assert.equal(await cards.count(),1);assert.equal(await cards.locator('[role=progressbar]').getAttribute('aria-valuenow'),'15');
  assert(!(await cards.textContent()).includes('TorBox to Unraid'));
  assert.equal(await page.evaluate(()=>serviceUsesLogin('torboxarr')),false);
  assert.equal(await page.locator('#serviceType option[value=torboxarr]').count(),1);
  assert.deepEqual(errors,[]);
  console.log('PASS: TorBoxarr local transfer, optional metrics, ARR deduplication, pending filter, Tdarr transition, responsive UI');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
