const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
  const page=await browser.newPage();
  const base=process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080';
  await page.route('https://overmynd.xivix.cc/**',async route=>{
   const u=new URL(route.request().url());
   const response=await page.request.get(base+u.pathname+u.search);
   await route.fulfill({response});
  });
  for(const host of ['xivix.cc','www.xivix.cc','untrusted.example']) {
   await page.route(`https://${host}/`,route=>route.fulfill({contentType:'text/html',body:'<iframe title="Overmynd" src="https://overmynd.xivix.cc/"></iframe>'}));
   await page.goto(`https://${host}/`);
   if(host==='untrusted.example') {
    await page.waitForLoadState('networkidle');
    assert.equal(await page.frameLocator('iframe').locator('#dashboardView').count(),0);
   } else {await page.frameLocator('iframe').locator('#dashboardView').waitFor();}
  }
  console.log('PASS: trusted portal iframe loads; unrelated framing blocked');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
