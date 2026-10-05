const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
  const browser = await chromium.launch({headless:true,...(process.env.BROWSER_CHANNEL?{channel:process.env.BROWSER_CHANNEL}:{})});
  try {
    const page = await browser.newPage({viewport:{width:1280,height:900}});
    const errors=[]; page.on('pageerror',e=>errors.push(e.message));
    const submitted=[]; const actions=[];
    let authenticated=false, saved;
    const pending={id:1,name:'Alice <script>alert(1)</script>',email:'alice@example.com',username:'alice',connect_username:'alice-connect',status:'pending'};
    await page.route('**/api/v1/**',route=>{
      const request=route.request(),path=new URL(request.url()).pathname;
      if(request.method()!=='GET') assert.equal(request.headers()['x-overmynd-request'],'1');
      if(path==='/api/v1/access/request'){submitted.push(request.postDataJSON());return route.fulfill({status:202,json:{message:'Submitted for approval. Watch your email.'}});}
      if(path==='/api/v1/access/setup'){submitted.push(request.postDataJSON());return route.fulfill({json:{message:'Your account is ready.'}});}
      if(path==='/api/v1/access/settings' && request.method()==='PUT'){saved=request.postDataJSON();return route.fulfill({json:{...saved,emby_key:undefined,smtp_password:undefined}});}
      if(path.startsWith('/api/v1/access/requests/')){actions.push(path);if(path.endsWith('/correct')) Object.assign(pending,request.postDataJSON());else if(path.endsWith('/reset')) {pending.status='pending';pending.emby_id='';}else {pending.status='awaiting_setup';pending.emby_id='created';}return route.fulfill({json:{message:'Request updated'}});}
      const values={
        '/api/v1/auth/status':{authenticated,setup_required:false,username:'admin'},
        '/api/v1/access/status':{enabled:true},
        '/api/v1/access/settings':{enabled:true,public_url:'https://overmynd.example',emby_url:'http://emby:8096',template_id:'template',smtp_host:'smtp.example',smtp_port:465,smtp_security:'tls',smtp_user:'mailbox',from:'access@example.com',admin_email:'admin@example.com',has_emby_key:true,has_smtp_password:true},
        '/api/v1/access/requests':{requests:[pending]},
        '/api/v1/access/test-email':{message:'Test email sent'},
        '/api/v1/media-request/status':{enabled:false},
        '/api/v1/request-settings':{}, '/api/v1/activity':{lifecycles:[]},
        '/api/v1/recently-added':{items:[]}, '/api/v1/downloads':{downloads:[]},
        '/api/v1/missing':{items:[]},'/api/v1/processing':{jobs:[]},'/api/v1/playback':{sessions:[]}
      };
      return route.fulfill({json:values[path]||[]});
    });
    await page.goto(process.env.OVERMYND_TEST_URL||'http://127.0.0.1:18080');
    await page.locator('#requestAccessButton').click();
    const form=page.locator('#accessRequestForm');
    await form.locator('[name=name]').fill('Alice');
    await form.locator('[name=email]').fill('alice@example.com');
    await form.locator('[name=username]').fill('alice');
    await form.locator('[name=connect_username]').fill('alice-connect');
    await form.locator('button').click();
    await page.locator('#accessRequestMessage').filter({hasText:'Submitted'}).waitFor();
    assert.deepEqual(submitted[0],{name:'Alice',email:'alice@example.com',username:'alice',connect_username:'alice-connect'});
    assert.equal(await page.locator('#accessAdminPanel').isVisible(),false);
    await page.locator('#accessRequestDialog [data-close-access]').click();
    await page.locator('#requestAccessButton').click();await page.mouse.click(1,1);
    assert.equal(await page.locator('#accessRequestDialog').isVisible(),false);
    const token='a'.repeat(64);
    await page.evaluate(token=>{location.hash=`access-setup=${token}`;},token);
    await page.locator('#accessSetupDialog').waitFor();assert.equal(new URL(page.url()).hash,'');
    const setup=page.locator('#accessSetupForm');
    await setup.locator('[name=password]').fill('a strong test password');
    await setup.locator('[name=confirm]').fill('a mismatched password');await setup.locator('button').click();
    assert.equal(submitted.length,1);
    await setup.locator('[name=confirm]').fill('a strong test password');await setup.locator('button').click();
    await page.locator('#accessSetupMessage').filter({hasText:'ready'}).waitFor();
    assert.deepEqual(submitted[1],{token,password:'a strong test password'});
    assert.equal(await setup.locator('[name=password]').inputValue(),'');
    await page.locator('#accessSetupDialog [data-close-access]').click();
    authenticated=true;await page.evaluate(()=>applyAuth({authenticated:true,username:'admin'}));
    await page.locator('[data-view=settings]').click();
    await page.locator('#accessQueue h4').waitFor();
    assert.equal(await page.locator('#accessQueue script').count(),0);
    await page.getByText('Access and email settings',{exact:true}).click();
    const settings=page.locator('#accessSettingsForm');
    assert.equal(await settings.locator('[name=emby_key]').inputValue(),'');
    await settings.locator('button[type=submit]').click();
    await page.locator('#accessAdminMessage').filter({hasText:'saved'}).waitFor();
    assert.equal(saved.smtp_port,465);assert.equal(saved.emby_key,'');
    await page.locator('#accessTestEmail').click();await page.locator('#accessAdminMessage').filter({hasText:'Test email sent'}).waitFor();
    await page.getByText('Correct contact details',{exact:true}).click();
    await page.locator('.access-correction [name=email]').fill('corrected@example.com');
    await page.getByRole('button',{name:'Save correction',exact:true}).click();
    await page.locator('#accessQueue > article > p').filter({hasText:'corrected@example.com'}).waitFor();
    await page.locator('#accessQueue button').filter({hasText:/^Approve$/}).click();
    await page.locator('#accessQueue button').filter({hasText:'fresh setup link'}).waitFor();
    assert.deepEqual(actions,['/api/v1/access/requests/1/correct','/api/v1/access/requests/1/approve']);
    await page.getByRole('button',{name:'Reapply template',exact:true}).click();
    await page.locator('#accessAdminMessage').filter({hasText:'Template settings verified'}).waitFor();
    assert.equal(actions.at(-1),'/api/v1/access/requests/1/reapply-template');
    await page.getByRole('button',{name:'Reset after Emby deletion',exact:true}).click();
    await page.locator('#accessQueue button').filter({hasText:/^Approve$/}).waitFor();
    assert.equal(actions.at(-1),'/api/v1/access/requests/1/reset');
    pending.status='active';
    await page.locator('#accessRefresh').click();
    await page.getByText('Completed requests (1)',{exact:true}).waitFor();
    assert.equal(await page.locator('.access-history').evaluate(n=>n.open),false);
    assert.equal(await page.locator('#accessQueue h4').isVisible(),false);
    await page.getByText('Completed requests (1)',{exact:true}).click();
    assert(await page.locator('#accessQueue h4').isVisible());
    pending.mail_error='Email delivery failed';
    await page.locator('#accessRefresh').click();
    await page.locator('#accessQueue > article .access-error').waitFor();
    assert.equal(await page.locator('.access-history').count(),0);
    await page.setViewportSize({width:390,height:844});
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    await page.locator('[data-view=dashboard]').click();await page.locator('#requestAccessButton').click();
    const bounds=await page.locator('#accessRequestDialog').boundingBox();assert(bounds.x>=0&&bounds.x+bounds.width<=390);
    await page.keyboard.press('Escape');
    authenticated=false;await page.evaluate(()=>applyAuth({authenticated:false}));
    assert.equal(await page.locator('#accessQueue').textContent(),'');assert.equal(await page.locator('#accessAdminPanel').isVisible(),false);
    assert.deepEqual(errors,[]);console.log('Access request, approval, setup, privacy and mobile checks passed');
  } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
