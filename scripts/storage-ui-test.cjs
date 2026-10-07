const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
  const page=await browser.newPage();
  let enabled=true;
  await page.route('**/api/v1/storage',route=>route.fulfill({json:{enabled,scanning:false,updated_at:new Date().toISOString(),free_bytes:13*1024**4,categories:[{name:'Movies',bytes:4*1024**4},{name:'TV',bytes:8*1024**4},{name:'Anime',bytes:1024**4},{name:'Music',bytes:0},{name:'Books',bytes:null}]}}));
  await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
  await page.locator('#storageSummary span').first().waitFor();
  assert.equal(await page.locator('#storageSummary span').count(),6);
  assert((await page.locator('#storageSummary').textContent()).includes('books: unavailable'));
  assert((await page.locator('#storageSummary').textContent()).includes('music: 0 B'));
  for(const width of [320,375,768,1440]) {
   await page.setViewportSize({width,height:1000});
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   assert(await page.locator('#storageSummary').evaluate(el=>{const b=el.getBoundingClientRect();return b.left>=0&&b.right<=innerWidth;}));
  }
  enabled=false;await page.reload();
  assert(await page.locator('#storageSummary').isHidden());
  console.log('PASS: public storage totals, missing/empty sizes, disabled state, responsive footer');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
