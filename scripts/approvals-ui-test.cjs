const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
 const page=await browser.newPage();
 let requests=[{id:'1',source_service_id:7,source_id:42,source:'seerr',status:'pending',title:'<script>test</script>',media_type:'tv'},{id:'2',source:'seerr',status:'approved',title:'Approved'}], errors=[];
 await page.route('**/api/v1/requests',r=>r.fulfill({json:{requests,errors}}));
 await page.goto(process.env.OVERMYND_TEST_URL || 'http://127.0.0.1:18080');
 await page.getByText('1 awaiting approval',{exact:true}).first().waitFor();
 for(const width of [320,768,1440]) {
 await page.setViewportSize({width,height:900});
 await page.locator('#pendingApprovals').click();
 await page.waitForFunction(()=>document.getElementById('approvalPopover').style.left!=='');
 assert.equal(await page.locator('#approvalList li').count(),1);
 assert.equal(await page.locator('#approvalList script').count(),0);
 assert(await page.locator('#approvalPopover').evaluate(el=>{let b=el.getBoundingClientRect();return b.left>=0&&b.right<=innerWidth}));
 await page.keyboard.press('Escape');
 }
 await page.locator('#pendingApprovals').click();
 assert(await page.getByRole('button',{name:'[ Approve ]',exact:true}).isHidden());
 await page.evaluate(()=>window.dispatchEvent(new CustomEvent('overmynd-auth',{detail:true})));
 await page.getByRole('button',{name:'[ Approve ]',exact:true}).waitFor();
 let decisions=0;
 await page.route('**/api/v1/requests/7/42/*', async route=>{
   assert.equal(route.request().method(),'POST');
   assert.equal(route.request().headers()['x-overmynd-request'],'1');
   decisions++;
   if(decisions===1) await route.fulfill({status:502,json:{error:'Seerr unavailable'}});
   else await route.fulfill({json:{ok:true}});
 });
 await page.getByRole('button',{name:'[ Approve ]',exact:true}).click();
 await page.getByText('Seerr unavailable',{exact:true}).waitFor();
 assert.equal(await page.locator('#approvalList li').count(),1);
 await page.evaluate(()=>window.dispatchEvent(new CustomEvent('overmynd-auth',{detail:false})));
 assert(await page.getByRole('button',{name:'[ Deny ]',exact:true}).isHidden());
 await page.evaluate(()=>window.dispatchEvent(new CustomEvent('overmynd-auth',{detail:true})));
 await page.getByRole('button',{name:'[ Deny ]',exact:true}).click();
 await page.waitForFunction(()=>document.getElementById('pendingApprovals').hidden);
 assert.equal(decisions,2);
 requests=[];await page.reload();await page.waitForFunction(()=>document.getElementById('approvalStatus').textContent==='No requests awaiting approval');
 assert(await page.locator('#pendingApprovals').isHidden());
 errors=[{error:'offline'}];await page.reload();await page.getByText('Approval status unavailable',{exact:true}).waitFor();
 console.log('PASS: pending approval count, escaping, responsive popover, clear and unavailable states');
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
