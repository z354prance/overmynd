const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
 try {
 const page=await browser.newPage();
 let requests=[{id:'1',source:'seerr',status:'pending',title:'<script>test</script>',media_type:'tv'},{id:'2',source:'seerr',status:'approved',title:'Approved'}], errors=[];
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
 requests=[];await page.reload();await page.waitForFunction(()=>document.getElementById('approvalStatus').textContent==='No requests awaiting approval');
 assert(await page.locator('#pendingApprovals').isHidden());
 errors=[{error:'offline'}];await page.reload();await page.getByText('Approval status unavailable',{exact:true}).waitFor();
 console.log('PASS: pending approval count, escaping, responsive popover, clear and unavailable states');
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
