const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
  const page=await browser.newPage();
  let enabled=true;
  await page.route('**/api/v1/storage',route=>route.fulfill({json:{enabled,scanning:false,updated_at:new Date().toISOString(),total_bytes:64*1024**4,free_bytes:13*1024**4,categories:[{name:'Movies',bytes:4*1024**4},{name:'TV',bytes:8*1024**4},{name:'Anime',bytes:1024**4},{name:'Music',bytes:0},{name:'Books',bytes:null}]}}));
  await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
  await page.locator('#storageSummary button').filter({hasText:'20.3% free'}).waitFor();
  await page.locator('#storageSummary button').click();
  assert(await page.locator('#storagePopover').isVisible());
  assert.equal(await page.locator('#storagePopover dl > div').count(),7);
  assert((await page.locator('#storagePopover').textContent()).includes('Booksunavailable'));
  assert((await page.locator('#storagePopover').textContent()).includes('Music0 B'));
  await page.keyboard.press('Escape');
  assert(await page.locator('#storagePopover').isHidden());
  for(const width of [320,375,768,1440]) {
   await page.setViewportSize({width,height:1000});
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   await page.locator('#storageSummary button').click();
   assert(await page.locator('#storagePopover').evaluate(el=>{const b=el.getBoundingClientRect();return b.left>=0&&b.right<=innerWidth&&b.top>=0&&b.bottom<=innerHeight;}));
   await page.waitForFunction(()=>{const p=document.getElementById('storagePopover');return p.matches(':popover-open') && p.style.left!=='';});
   await page.mouse.click(1,1);
   await page.waitForFunction(()=>!document.getElementById('storagePopover').matches(':popover-open'));
   assert(await page.locator('#storagePopover').isHidden());
   assert(await page.locator('#storageSummary').evaluate(el=>{const b=el.getBoundingClientRect();return b.left>=0&&b.right<=innerWidth;}));
  }
  enabled=false;await page.reload();
  assert(await page.locator('#storageSummary').isHidden());
  console.log('PASS: public storage totals, missing/empty sizes, disabled state, responsive footer');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
