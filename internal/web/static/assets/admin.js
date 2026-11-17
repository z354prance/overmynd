(function(){
  var root=document.getElementById('root'),logoutBtn=document.getElementById('logout'),timer=null;
  function el(t,c,x){var e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e}
  function api(method,path,body){
    return fetch('/admin/api'+path,{method:method,headers:{'Content-Type':'application/json','X-Undermynd-Request':'1'},body:body?JSON.stringify(body):undefined,credentials:'same-origin'})
      .then(function(r){return r.json().catch(function(){return {}}).then(function(d){if(!r.ok){var e=new Error(d.error||('HTTP '+r.status));e.status=r.status;throw e}return d})});
  }
  function field(form,label,type,name,ph){form.appendChild(el('label','',label));var i=el('input');i.type=type||'text';i.name=name;if(ph)i.placeholder=ph;i.autocomplete=type==='password'?'current-password':'off';form.appendChild(i);return i}
  function prompt(cmd){var d=el('div');d.appendChild(el('span','prompt','~/overmynd '));var b=el('b','','❯');b.style.color='var(--teal)';b.style.fontWeight='400';d.appendChild(b);d.appendChild(el('span','cmd',' '+cmd));return d}
  function start(){
    stopLive();logoutBtn.hidden=true;document.body.classList.remove('wide');
    api('GET','/session').then(function(s){
      if(s.signedIn)return app();
      if(s.needsSetup)return setup();
      if(s.pending)return confirmStep();
      login();
    }).catch(function(e){root.replaceChildren(el('div','err',e.message))});
  }
  function login(){
    root.replaceChildren(prompt('sign-in'));
    var f=el('form','form');var p=field(f,'password','password','password');var c=field(f,'authenticator code (or a recovery code)','text','code');c.autocomplete='one-time-code';
    var b=el('button','go','sign in');b.type='submit';f.appendChild(el('div'));f.appendChild(b);var err=el('div','err');f.appendChild(err);
    f.onsubmit=function(e){e.preventDefault();err.textContent='';api('POST','/login',{password:p.value,code:c.value}).then(start).catch(function(x){err.textContent=x.message})};
    root.appendChild(f);p.focus();
  }
  var pendingPw='';
  function setup(){
    root.replaceChildren(prompt('setup'));
    root.appendChild(el('p','hint','First run. The setup key is printed in the container log (also in the config folder as setup-key).'));
    var f=el('form','form');var k=field(f,'setup key','text','key');var p=field(f,'choose a password (12+ characters)','password','password');p.autocomplete='new-password';
    var b=el('button','go','create account');b.type='submit';f.appendChild(el('div'));f.appendChild(b);var err=el('div','err');f.appendChild(err);
    f.onsubmit=function(e){e.preventDefault();err.textContent='';api('POST','/setup',{key:k.value,password:p.value}).then(function(r){pendingPw=p.value;confirmStep(r)}).catch(function(x){err.textContent=x.message})};
    root.appendChild(f);k.focus();
  }
  function confirmStep(r){
    root.replaceChildren(prompt('setup --authenticator'));
    if(r){
      root.appendChild(el('p','hint','Add this secret to an authenticator app (any TOTP app), then enter the 6-digit code it shows.'));
      root.appendChild(el('pre','',r.totpSecret));
      root.appendChild(el('div','hint','or open this link on your phone: '));
      var a=el('a','',r.otpauthUri);a.href=r.otpauthUri;root.appendChild(a);
    }else{root.appendChild(el('p','hint','Setup was started but not finished. Enter your password and the code from your authenticator.'))}
    var f=el('form','form');var p=null;if(!pendingPw)p=field(f,'password','password','password');var c=field(f,'code from the app','text','code');c.autocomplete='one-time-code';
    var b=el('button','go','confirm');b.type='submit';f.appendChild(el('div'));f.appendChild(b);var err=el('div','err');f.appendChild(err);
    f.onsubmit=function(e){e.preventDefault();err.textContent='';api('POST','/setup/confirm',{password:pendingPw||p.value,code:c.value}).then(function(d){pendingPw='';
      root.replaceChildren(prompt('recovery-codes'));root.appendChild(el('p','hint','Save these now. Each works once if you lose your authenticator. They are not shown again.'));
      root.appendChild(el('pre','',d.recovery.join('\n')));var go=el('button','go','continue to sign in');go.onclick=start;root.appendChild(go)}).catch(function(x){err.textContent=x.message})};
    root.appendChild(f);c.focus();
  }
  // ---------------------------------------------------------------- app
  var tab='overview',detailId=null;
  function app(){
    logoutBtn.hidden=false;logoutBtn.onclick=function(){api('POST','/logout').then(start)};
    root.replaceChildren();
    document.body.classList.add('wide');
    var shell=el('div','shell'),side=el('nav','side');
    var brand=el('a','brand');brand.href='/admin';var bi=el('img');bi.src='/assets/logo.svg';bi.alt='';brand.appendChild(bi);brand.appendChild(el('span','',  '~/overmynd'));side.appendChild(brand);
    function nav(t,label){var b=el('button',t===tab?'on':'',label);tabBtns[t]=b;b.onclick=function(){stopLive();tab=t;detailId=null;app()};side.appendChild(b)}
    nav('overview','Overview');nav('jobs','Jobs');nav('requests','Requests');nav('library','Library');nav('reencode','Re-encode');
    [['Connections',[['arrs','Arrs & categories'],['clients','Download clients'],['transcode','Transcode']]],['Public site',[['playback','Playback'],['public','Public page']]],['Automation',[['rss','Feeds']]],['Server',[['system','Settings'],['log','Log']]]].forEach(function(g){
      side.appendChild(el('div','sgroup',g[0]));
      g[1].forEach(function(p){nav(p[0],p[1])})});
    side.appendChild(el('span','sp'));var pub=el('a','sidelink','public page');pub.href='/';side.appendChild(pub);
    var so=el('button','sidelink','sign out');so.onclick=function(){api('POST','/logout').then(start)};side.appendChild(so);
    var main=el('main','main'),body=el('div');main.appendChild(body);shell.appendChild(side);shell.appendChild(main);root.appendChild(shell);
    if(tab==='overview')overviewView(body);else if(tab==='jobs')jobsView(body);else if(tab==='requests')requestsView(body);else if(tab==='library')libraryView(body);else if(tab==='reencode')reencodeView(body);else if(tab==='log')logView(body);else settingsView(body);
  }
  function fail(e){if(e.status===401){start()}else{var d=el('div','err',e.message);root.appendChild(d)}}
  function ago(t){var s=Math.max(0,(Date.now()-new Date(t))/1000);if(s<90)return Math.round(s)+'s';if(s<5400)return Math.round(s/60)+'m';if(s<172800)return Math.round(s/3600)+'h';return Math.round(s/86400)+'d'}
  var jobFilter={q:'',group:'all',source:'all'},lastRows=[],paused=false,es=null,tabBtns={};
  function stopLive(){clearInterval(timer);if(es){es.close();es=null}}
  function attentionCount(rows){return rows.filter(function(r){return r.attention&&r.state!=='cleaned'}).length}
  var WTORBOX=['Requested','Found','Download','Transfer','Convert','Ready'],WLOCAL=['Requested','Found','Download','Convert','Ready'];
  function jobWire(j){
    // A local downloader writes straight to the server: no Transfer step.
    var tb=j.backend==='torbox',WSTAGES=tb?WTORBOX:WLOCAL,ix=function(n){return WSTAGES.indexOf(n)};
    var fa={download:'Download',transfer:tb?'Transfer':'Download',convert:'Convert',ready:'Ready'}[j.failedAt]||'Download';
    var m={received:[0,'wait'],submitted:[1,'wait'],downloading:[ix('Download'),'run'],pulling:[ix(tb?'Transfer':'Download'),'run'],downloaded:[ix('Convert'),'wait'],queued_transcode:[ix('Convert'),'wait'],transcoding:[ix('Convert'),'run'],verifying:[ix('Convert'),'run'],ready:[ix('Ready'),'wait'],imported:[WSTAGES.length,'done'],cleaned:[WSTAGES.length,'done'],failed:[ix(fa),'fail']}[j.state]||[1,'wait'];
    var cur=m[0],mode=m[1],pct=Math.max(0,Math.min(100,Math.round((j.progress||0)*100)));
    if(mode==='run'&&pct===0)mode='wait';
    var w=el('div','wire w-'+mode);
    WSTAGES.forEach(function(name,i){
      var st=el('div','st '+(cur>i?'done':cur===i?'cur':'todo'));var wr=el('div','wr');wr.appendChild(el('i','nd'));
      if(i<WSTAGES.length-1){
        var ln=el('b','ln'+(cur>i?'':cur===i?' act':' todo'));
        if(cur===i){var p=(mode==='run'||mode==='fail')?pct:0;ln.style.setProperty('--p',p+'%');var f=el('s','fill');f.style.width=p+'%';f.style.backgroundSize=(p>0?Math.round(10000/p):100)+'% 100%';ln.appendChild(f);if(mode==='run'&&p>0){var d=el('u','dot');d.style.left=p+'%';ln.appendChild(d)}ln.appendChild(el('em','rest'))}
        wr.appendChild(ln)}
      st.appendChild(wr);st.appendChild(el('span','lb',name));w.appendChild(st)});
    return w}
  function jobActions(j){
    var act=el('div','acts');
    function b(cls,label,fn,title){var x=el('button',cls,label);if(title)x.title=title;x.onclick=fn;act.appendChild(x)}
    if(j.state==='failed'){b('go','retry',function(){doAction(j.id,'retry')});b('','clean up now',function(){doAction(j.id,'cleanup')})}
    if(j.state==='ready')b('go','mark imported',function(){doAction(j.id,'imported')});
    if(j.state==='imported')b('','clean up now',function(){doAction(j.id,'cleanup')});
    if(j.state!=='cleaned'&&j.state!=='failed'&&j.state!=='imported')b('','fail it',function(){if(confirm('Fail this job? The arr will treat the download as failed and may search again.'))doAction(j.id,'fail')},'report a failed download so the arr blocklists this release and grabs another');
    if(j.state!=='cleaned')b('bad','cancel',function(){if(confirm('Cancel and clean up this job?'))doAction(j.id,'cancel')});
    return act}
  function jobRow(j,onOpen){
    var r=el('div','jrow'+(j.state==='failed'?' failed':''));
    var n=el('div','jname');var a=el('button','link',j.name);a.onclick=function(){onOpen(j)};n.appendChild(a);
    var why=(j.flag?j.flag+' ':'')+(j.reason&&j.state!=='cleaned'?j.reason:'');
    n.appendChild(el('div','reason',why||(j.source+' / '+j.category+' · '+ago(j.created)+' ago')));
    if(why)n.appendChild(el('div','reason faintline',j.source+' / '+j.category+' · '+(j.backend||'-')+' · '+ago(j.created)+' ago'+(j.transcodeNode?' · '+j.transcodeNode+(j.transcodeWorker?' / '+j.transcodeWorker:''):'')));
    r.appendChild(n);var w=el('div','jwire');w.appendChild(jobWire(j));r.appendChild(w);r.appendChild(jobActions(j));return r}
  var WORKING={queued:1,downloading:1,processing:1,finishing:1};
  function jobsView(body){
    body.appendChild(el('h2','ptitle','Jobs'));
    var bar=el('div','toolbar');var pills=el('div','pills');bar.appendChild(pills);
    var q=el('input');q.placeholder='search title';q.value=jobFilter.q;q.oninput=function(){jobFilter.q=q.value;drawRows()};bar.appendChild(q);
    var ss=el('select');[['all','all sources'],['sonarr','sonarr'],['radarr','radarr'],['lidarr','lidarr'],['rss','rss']].forEach(function(x){var o=el('option','',x[1]);o.value=x[0];if(jobFilter.source===x[0])o.selected=true;ss.appendChild(o)});ss.onchange=function(){jobFilter.source=ss.value;drawRows()};bar.appendChild(ss);
    bar.appendChild(el('span','sp'));
    var pb=el('button','','');function pauseLabel(){pb.textContent=paused?'resume intake':'pause intake';pb.className=paused?'go':''}
    pb.onclick=function(){api('POST','/intake',{paused:!paused}).then(function(r){paused=r.paused;pauseLabel();banner()}).catch(fail)};pauseLabel();bar.appendChild(pb);
    body.appendChild(bar);var storageUnavailable=false;var note=el('div','notice');body.appendChild(note);var host=el('div');body.appendChild(host);
    function banner(){note.textContent=storageUnavailable?'Job storage is unavailable. Showing the last received jobs.':paused?'Intake is paused: new grabs are accepted but held until you resume.':'';note.style.display=(paused||storageUnavailable)?'block':'none';pauseLabel()}
    function inGroup(r,g){
      if(g==='attention')return r.attention&&r.state!=='cleaned';
      if(g==='working')return !!WORKING[r.group];
      if(g==='done')return r.group==='done';
      return r.state!=='cleaned'}
    function match(r){
      if(jobFilter.q&&r.name.toLowerCase().indexOf(jobFilter.q.toLowerCase())<0)return false;
      if(jobFilter.source!=='all'&&r.source!==jobFilter.source)return false;
      return inGroup(r,jobFilter.group)}
    function drawRows(){
      if(tab!=='jobs'||detailId)return;
      var base=lastRows.filter(function(r){return (!jobFilter.q||r.name.toLowerCase().indexOf(jobFilter.q.toLowerCase())>=0)&&(jobFilter.source==='all'||r.source===jobFilter.source)});
      pills.replaceChildren();
      [['all','All'],['attention','Needs you'],['working','Working'],['done','Done']].forEach(function(g){
        var n=base.filter(function(r){return inGroup(r,g[0])}).length;
        var p=el('button','fpill '+g[0]+(jobFilter.group===g[0]?' on':''),g[1]+' '+n);p.onclick=function(){jobFilter.group=g[0];drawRows()};pills.appendChild(p)});
      var list=lastRows.filter(match);host.replaceChildren();
      var ac=attentionCount(lastRows);if(tabBtns.jobs){tabBtns.jobs.textContent='Jobs';if(ac){var bd=el('span','badge',String(ac));bd.title=ac+' need attention';tabBtns.jobs.appendChild(bd)}}
      if(!list.length){host.appendChild(el('div','empty',lastRows.length?'nothing matches these filters':'no jobs yet'));return}
      var box=el('div','jlist');list.forEach(function(j){box.appendChild(jobRow(j,function(j){detailId=j.id;detail(body,j.id)}))});host.appendChild(box);
    }
    function take(d){storageUnavailable=false;lastRows=d.jobs||[];paused=!!d.paused;banner();drawRows()}
    function poll(){api('GET','/jobs').then(function(rows){lastRows=rows;drawRows()}).catch(fail)}
    if(window.EventSource){
      es=new EventSource('/admin/api/events');
      es.addEventListener('storage-error',function(){storageUnavailable=true;banner()});
      es.addEventListener('jobs',function(e){try{take(JSON.parse(e.data))}catch(x){}});
      es.onerror=function(){if(es&&es.readyState===2){es.close();es=null;poll();timer=setInterval(poll,3000)}};
    }else{poll();timer=setInterval(poll,3000)}
    banner();drawRows();
  }

  function loadReqs(){return api('GET','/requests').then(function(d){return d.requests||[]}).catch(function(){return null})}
  function decideReq(r,action,done){api('POST','/requests/'+encodeURIComponent(r.source_id)+'/'+action).then(done).catch(function(e){if(e.status===401)fail(e);else{alert(e.message);done()}})}
  function reqRow(r,done,compact){
    var row=el('div',compact?'orow rrow':'jrow rrow');
    var n=el('div','jname');n.appendChild(el('div','',(r.title||'Untitled request')));
    var who=[r.media_type==='tv'?'TV show':'Movie',r.requested_by,r.seasons&&r.seasons.length?'seasons '+r.seasons.join(', '):''].filter(Boolean).join(' · ');
    n.appendChild(el('div','reason',who));row.appendChild(n);
    var a=el('div','acts rhoriz');var ap=el('button','go','Approve');ap.onclick=function(){ap.disabled=true;decideReq(r,'approve',done)};var dc=el('button','','Decline');dc.onclick=function(){dc.disabled=true;decideReq(r,'decline',done)};a.appendChild(ap);a.appendChild(dc);row.appendChild(a);return row}
  function setReqBadge(n){if(!tabBtns.requests)return;tabBtns.requests.textContent='Requests';if(n){var bd=el('span','badge',String(n));tabBtns.requests.appendChild(bd)}}
  function logView(body){
    body.appendChild(el('h2','ptitle','Log'));
    body.appendChild(el('p','hint','The container log since Overmynd last started, newest at the bottom. Type a job id (the short code after "job"), or words like tdarr or failed, to narrow it. The last 2000 lines are kept.'));
    var bar=el('div','rebar');var q=el('input');q.placeholder='filter';q.style.minWidth='220px';bar.appendChild(q);
    var fl=el('label','','');var follow=el('input');follow.type='checkbox';follow.checked=true;fl.appendChild(follow);fl.appendChild(document.createTextNode(' follow'));bar.appendChild(fl);
    var info=el('span','msg');bar.appendChild(info);body.appendChild(bar);
    var pre=el('pre','logbox');pre.style.cssText='max-height:70vh;overflow:auto;white-space:pre-wrap;word-break:break-all;color:var(--fg,inherit);margin-top:14px;font-size:12px;line-height:1.45';body.appendChild(pre);
    var busy=false;
    function load(){
      if(busy)return;busy=true;
      api('GET','/log?n=500&q='+encodeURIComponent(q.value.trim())).then(function(r){
        var stick=follow.checked||pre.scrollTop+pre.clientHeight>=pre.scrollHeight-8;
        pre.textContent=r.lines.length?r.lines.join('\n'):'No lines match.';
        info.textContent=r.lines.length+' lines';
        if(stick)pre.scrollTop=pre.scrollHeight
      }).catch(fail).then(function(){busy=false})
    }
    q.oninput=load;load();timer=setInterval(function(){if(follow.checked)load()},3000)
  }
  function reencodeView(body){
    body.appendChild(el('h2','ptitle','Re-encode'));
    body.appendChild(el('p','hint','Finds videos in your library that are not AV1 and arrived since a date, so files that were imported before Tdarr finished can go back through it. Each one is copied, encoded and checked as AV1, and only then written over the library file. The original is left alone if anything fails.'));
    var bar=el('div','rebar');
    var d=new Date();d.setDate(d.getDate()-1);d.setHours(8,0,0,0);
    function local(dt){var z=new Date(dt.getTime()-dt.getTimezoneOffset()*60000);return z.toISOString().slice(0,16)}
    var lbl=el('label','','Added since ');var since=el('input');since.type='datetime-local';since.value=local(d);lbl.appendChild(since);bar.appendChild(lbl);
    var scan=el('button','go','Scan library');bar.appendChild(scan);body.appendChild(bar);
    var host=el('div');host.style.marginTop='16px';body.appendChild(host);
    var files=[],picked={};
    function size(n){if(!n)return '0 B';var u=['B','KB','MB','GB','TB'],i=0;while(n>=1024&&i<4){n/=1024;i++}return n.toFixed(i?1:0)+' '+u[i]}
    function draw(msg){
      host.replaceChildren();if(msg){host.appendChild(el('div','reason',msg))}
      if(!files.length){if(!msg)host.appendChild(el('div','empty','Nothing found. Scan to look.'));return}
      var open=files.filter(function(f){return !f.queued});var n=Object.keys(picked).filter(function(k){return picked[k]}).length;
      var head=el('div','rehead');var all=el('input');all.type='checkbox';all.checked=open.length>0&&n===open.length;all.onchange=function(){open.forEach(function(f){picked[f.path]=all.checked});draw(msg)};
      var al=el('label','','');al.appendChild(all);al.appendChild(document.createTextNode(' '+files.length+' not AV1 · '+n+' selected'));head.appendChild(al);
      var q=el('button','go','Queue '+n+' for re-encode');q.disabled=!n;q.onclick=function(){q.disabled=true;
        api('POST','/reencode',{paths:Object.keys(picked).filter(function(k){return picked[k]})}).then(function(r){
          files.forEach(function(f){if(picked[f.path])f.queued=true});picked={};
          draw('Queued '+r.queued+(r.skipped&&r.skipped.length?', skipped '+r.skipped.length+': '+r.skipped.slice(0,3).join('; '):'')+'. Follow them under Jobs.')}).catch(function(e){q.disabled=false;draw(e.message)})};
      head.appendChild(q);host.appendChild(head);
      var box=el('div','jlist');
      files.forEach(function(f){var row=el('div','jrow rerow');var c=el('input');c.type='checkbox';c.disabled=f.queued;c.checked=!!picked[f.path];c.onchange=function(){picked[f.path]=c.checked;draw(msg)};
        var nm=el('div','jname');nm.appendChild(el('div','',f.rel));
        nm.appendChild(el('div','reason',[f.codec==='unknown'?'codec unknown'+(f.error?' ('+f.error+')':''):f.codec,size(f.size),'added '+ago(f.changed)+' ago',f.queued?'queued':''].filter(Boolean).join(' · ')));
        row.appendChild(c);row.appendChild(nm);box.appendChild(row)});
      host.appendChild(box)}
    scan.onclick=function(){scan.disabled=true;picked={};host.replaceChildren(el('div','reason','Scanning the library. This can take a minute.'));
      api('GET','/reencode?since='+encodeURIComponent(new Date(since.value).toISOString())).then(function(r){files=r.files||[];scan.disabled=false;draw(files.length?'':'Everything added since then is AV1.')}).catch(function(e){scan.disabled=false;if(e.status===401){start()}else{host.replaceChildren(el('div','err',e.message))}})};
    draw();
  }
  function requestsView(body){
    body.appendChild(el('h2','ptitle','Requests'));var host=el('div');host.style.marginTop='18px';body.appendChild(host);
    function draw(){loadReqs().then(function(list){host.replaceChildren();
      if(list===null){host.appendChild(el('div','empty','Requests are not set up, or Seerr could not be reached. Set it under Public page.'));return}
      setReqBadge(list.length);
      if(!list.length){host.appendChild(el('div','empty','Nothing is waiting for approval.'));return}
      var box=el('div','jlist');list.forEach(function(r){box.appendChild(reqRow(r,draw,false))});host.appendChild(box)})}
    draw();timer=setInterval(draw,15000)}
  function libraryView(body){
    body.appendChild(el('h2','ptitle','Library'));var host=el('div');host.style.marginTop='18px';body.appendChild(host);
    api('GET','/library').then(function(d){
      var errs=d.errors||[];if(errs.length){host.appendChild(el('h3','osec','Problems · '+errs.length));var c=el('div','ncard');errs.slice(0,10).forEach(function(e){c.appendChild(el('div','reason',e))});host.appendChild(c)}
      var seasons=d.seasons||[],shows={};seasons.forEach(function(x){(shows[x.showId]=shows[x.showId]||{name:x.Show||x.show,list:[]}).list.push(x)});
      var ids=Object.keys(shows).sort(function(a,b){return shows[a].name.localeCompare(shows[b].name)});
      host.appendChild(el('h3','osec','Series · '+ids.length));
      if(!ids.length)host.appendChild(el('div','empty','No series are being tracked.'));
      var box=el('div','jlist');
      ids.forEach(function(id){var sh=shows[id],imp=0,tot=0;sh.list.sort(function(a,b){return a.season-b.season});sh.list.forEach(function(x){imp+=x.imported;tot+=x.total});
        var row=el('div','jrow lrow');var n=el('div','jname');n.appendChild(el('div','',sh.name));n.appendChild(el('div','reason',imp+' / '+tot+' episodes imported'));row.appendChild(n);
        var cells=el('div','lcells');sh.list.forEach(function(x){var pct=x.total?100*x.imported/x.total:0;var c=el('span','lc '+(x.total&&x.imported>=x.total?'done':x.imported?'part':'none'));c.title='Season '+x.season+': '+x.imported+' / '+x.total;var f=el('i');f.style.height=pct+'%';c.appendChild(f);c.appendChild(el('b','',String(x.season)));cells.appendChild(c)});row.appendChild(cells);box.appendChild(row)});
      host.appendChild(box);
      var miss=d.missing||[];host.appendChild(el('h3','osec','Missing · '+miss.length));
      if(!miss.length)host.appendChild(el('div','empty','Nothing is missing.'));else{var mb=el('div','jlist');miss.slice(0,60).forEach(function(m){var r=el('div','jrow lrow');var n=el('div','jname');n.appendChild(el('div','',m.title));n.appendChild(el('div','reason',[m.kind,m.detail,m.category].filter(Boolean).join(' · ')));r.appendChild(n);mb.appendChild(r)});host.appendChild(mb)}
    }).catch(fail)}
  function overviewView(body){
    var head=el('div','ohead');head.appendChild(el('h2','ptitle','Overview'));var upd=el('span','reason','');head.appendChild(upd);body.appendChild(head);
    var chips=el('div','chips');body.appendChild(chips);
    var cols=el('div','ocols');var left=el('div'),right=el('div');cols.appendChild(left);cols.appendChild(right);body.appendChild(cols);
    var reqHost=el('div');var sysInfo=null,reqList=null;
    function drawChips(){chips.replaceChildren();if(!sysInfo)return;
      function chip2(t,st){chips.appendChild(el('span','chip2 '+(st||'ok'),t))}
      chip2(sysInfo.paused?'Intake paused':'Intake running',sysInfo.paused?'warn':'ok');
      if(sysInfo.torbox.configured)chip2(sysInfo.torbox.backoffUntil?'TorBox backing off':'TorBox '+sysInfo.torbox.used+' of '+sysInfo.torbox.limit+' this hour',sysInfo.torbox.backoffUntil?'warn':'ok');
      chip2(sysInfo.tdarr.api?'Tdarr '+sysInfo.tdarr.inFlight+' of '+sysInfo.tdarr.max+' busy':'Tdarr: folders only',sysInfo.tdarr.api?'ok':'dim');
      if(sysInfo.complete&&sysInfo.complete.totalBytes){var fr=sysInfo.complete.freeBytes/sysInfo.complete.totalBytes;chip2('Complete folder '+Math.round(fr*100)+'% free',fr<.1?'bad':'ok')}}
    function drawReqs(){reqHost.replaceChildren();if(reqList===null)return;setReqBadge(reqList.length);var h=el('h3','osec','Requests · '+reqList.length+' waiting');h.style.marginTop='22px';reqHost.appendChild(h);
      if(!reqList.length){reqHost.appendChild(el('div','empty','Nothing is waiting.'));return}
      var box=el('div','okv');reqList.slice(0,5).forEach(function(r){box.appendChild(reqRow(r,pollReqs,true))});reqHost.appendChild(box)}
    function pollReqs(){loadReqs().then(function(l){reqList=l;drawReqs()})}
    function draw(){
      drawChips();drawReqs();
      upd.textContent='Updated '+new Date().toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
      var need=lastRows.filter(function(r){return r.attention&&r.state!=='cleaned'});
      left.replaceChildren();left.appendChild(el('h3','osec','Needs you · '+need.length));
      if(!need.length)left.appendChild(el('div','empty','Nothing needs you right now.'));
      need.forEach(function(j){
        var c=el('div','ncard');var h=el('div','nh');var a=el('button','link',j.name);a.onclick=function(){tab='jobs';detailId=j.id;app()};h.appendChild(a);h.appendChild(el('span','fpill failed',j.state==='failed'?'Failed':'Attention'));c.appendChild(h);
        c.appendChild(el('div','reason',(j.flag?j.flag+' ':'')+(j.reason||'')));c.appendChild(jobActions(j));left.appendChild(c)});
      right.replaceChildren();right.appendChild(el('h3','osec','Pipeline now'));
      var cnt=function(f){return lastRows.filter(f).length};
      var box=el('div','okv');
      [['Downloading',cnt(function(r){return r.state==='downloading'||r.state==='pulling'})],['Waiting for a converter',cnt(function(r){return r.state==='queued_transcode'})],['Converting',cnt(function(r){return r.state==='transcoding'||r.state==='verifying'})],['Waiting on the arr',cnt(function(r){return r.state==='ready'})],['Finished',cnt(function(r){return r.group==='done'})]].forEach(function(x){var r=el('div','orow');r.appendChild(el('span','',x[0]));r.appendChild(el('b','',String(x[1])));box.appendChild(r)});
      right.appendChild(box);
      right.appendChild(reqHost);
      var ac=attentionCount(lastRows);if(tabBtns.jobs){tabBtns.jobs.textContent='Jobs';if(ac){var bd=el('span','badge',String(ac));tabBtns.jobs.appendChild(bd)}}
    }
    function poll(){api('GET','/jobs').then(function(d){lastRows=Array.isArray(d)?d:(d.jobs||[]);draw()}).catch(fail)}
    function pollSys(){api('GET','/system').then(function(d){sysInfo=d;drawChips()}).catch(function(){})}
    poll();pollSys();pollReqs();timer=setInterval(function(){poll();pollSys();pollReqs()},5000);
  }
  function doAction(id,a){api('POST','/jobs/'+encodeURIComponent(id)+'/'+a).then(function(){app()}).catch(function(e){if(e.status===401)fail(e);else{alert(e.message);app()}})}
  function detail(body,id){
    stopLive();body.replaceChildren();
    var back=el('button','link','← all jobs');back.onclick=function(){detailId=null;app()};body.appendChild(back);
    api('GET','/jobs/'+encodeURIComponent(id)).then(function(j){
      body.appendChild(el('h3','',j.name));
      var c=el('div','card');[['state',j.state],['reason',j.reason||'-'],['id (download id)',j.id],['category',j.category],['source',j.source],['backend',j.backend||'-'],['complete folder',j.completeDir||'-'],['files',(j.files||[]).join(', ')||'-']].forEach(function(p){var d=el('div');d.appendChild(el('span','prompt',p[0]+': '));d.appendChild(document.createTextNode(p[1]));c.appendChild(d)});
      body.appendChild(c);body.appendChild(el('div','sec','── history '));
      var t=el('table');(j.history||[]).slice().reverse().forEach(function(h){var r=el('tr');r.appendChild(el('td','mono',new Date(h.at).toLocaleString()));r.appendChild(el('td','',(h.from||'·')+' → '+h.to));r.appendChild(el('td','reason',h.reason||''));t.appendChild(r)});body.appendChild(t);
    }).catch(fail);
  }
  // ------------------------------------------------------------ settings
  var S=null;
  function clone(o){return JSON.parse(JSON.stringify(o))}
  // save sends the whole settings object every time, so no tab can wipe another tab's data.
  // Secrets are never sent back unless typed: a blank secret means "unchanged".
  function save(mut,msg){
    var b=clone(S);mut(b);msg.className='msg';msg.textContent='saving…';
    return api('PUT','/settings',b).then(function(r){S=r;msg.className='msg ok';msg.textContent='saved';setTimeout(function(){if(msg.textContent==='saved')msg.textContent=''},3000)}).catch(function(e){msg.className='msg err';msg.textContent=e.message})
  }
  function help(t){return el('p','hint',t)}
  function head(title,sub){var d=el('div','pane-head');d.appendChild(el('h3','',title));if(sub)d.appendChild(el('p','hint',sub));return d}
  // one labelled input with help text; returns the input
  function row(box,label,opts){
    opts=opts||{};var w=el('div','fld');w.appendChild(el('label','',label));
    var i=el('input');i.type=opts.type||'text';i.value=opts.value==null?'':opts.value;i.autocomplete='off';if(opts.ph)i.placeholder=opts.ph;if(opts.type==='checkbox'){i.checked=!!opts.value;i.style.width='auto'}
    w.appendChild(i);if(opts.help)w.appendChild(el('div','fh',opts.help));box.appendChild(w);return i
  }
  function chip(on,text){return el('span','chip2 '+(on?'ok':'dim'),text)}
  function saveBar(body,fn){var bar=el('div','savebar');var b=el('button','go','save');var m=el('span','msg');b.onclick=function(){fn(m)};bar.appendChild(b);bar.appendChild(m);body.appendChild(bar)}

  function settingsView(body){
    api('GET','/settings').then(function(s0){S=s0;pane(body)}).catch(fail)
  }
  function pane(body){
    body.replaceChildren();
    ({arrs:arrsPane,clients:clientsPane,transcode:transcodePane,playback:playbackPane,public:publicPane,rss:rssPane,system:systemPane})[tab](body);
    if(tab!=='system'&&tab!=='rss')split(body);
  }
  // A settings card becomes a row: its title and explanation on the left, the fields on the right.
  function split(body){
    [].filter.call(body.children,function(c){return c.classList.contains('card')&&c.firstChild&&c.firstChild.tagName==='H4'}).forEach(function(c){
      var l=el('div','sl'),r=el('div','sr');l.appendChild(c.firstChild);
      if(c.firstChild&&c.firstChild.classList&&c.firstChild.classList.contains('hint'))l.appendChild(c.firstChild);
      while(c.firstChild)r.appendChild(c.firstChild);c.classList.add('setting');c.appendChild(l);c.appendChild(r)})
  }

  function arrsPane(body){
    body.appendChild(head('Arrs & categories','Each category is a label an arr sends when it grabs something. It decides which arr confirms the import, and which folder the finished download lands in.'));
    var conn=el('div','card');conn.appendChild(el('h4','','Connect an arr to Overmynd'));
    var host=location.hostname;
    conn.appendChild(help('In Sonarr, Radarr or Lidarr: Settings → Download Clients → add. Use SABnzbd for usenet and qBittorrent for torrents.'));
    [['host',host],['port','8081 (the LAN-only machine port)'],['SABnzbd API key','the client password (see the System tab)'],['qBittorrent login','username '+S.clientUser+' and the client password'],['category','one of the names below, exactly']].forEach(function(p){var d=el('div','kv');d.appendChild(el('span','k',p[0]));d.appendChild(el('span','v',p[1]));conn.appendChild(d)});
    body.appendChild(conn);
    var cats=el('div');body.appendChild(cats);
    function add(c){
      var card=el('div','card cat kind-'+(c.kind||'none'));var f={};var g=el('div','grid3');f.orig=c.name||'';
      f.name=row(g,'category name',{value:c.name,help:'what the arr sends, e.g. tv'});
      var w=el('div','fld');w.appendChild(el('label','','arr type'));var k=el('select');[['sonarr','sonarr'],['radarr','radarr'],['lidarr','lidarr'],['rss','no arr (RSS / watch folder)']].forEach(function(p){var x=p[0];var o=el('option','',p[1]);o.value=x;if(c.kind===x)o.selected=true;k.appendChild(o)});w.appendChild(k);g.appendChild(w);f.kind=k;
      f.arrUrl=row(g,'arr address',{value:c.arrUrl,ph:'http://10.0.0.5:8989',help:'so Overmynd can confirm the import'});
      f.arrApiKey=row(g,'arr API key',{type:'password',ph:c.hasArrApiKey?'set — leave blank to keep':'',help:'in the arr: Settings → General'});
      f.transcode=row(g,'transcode with Tdarr',{type:'checkbox',value:c.transcode,help:'off for music'});
      f.libraryDir=row(g,'library folder',{value:c.libraryDir,ph:'/media/Anime',help:'only for “no arr” categories: finished episodes are matched to a show folder here and renamed'});
      var pw=el('div','fld');pw.appendChild(el('label','','if the transcode fails'));var pf=el('select');[['fail','fail the grab (the arr blocklists and searches again)'],['passthrough','keep the original file']].forEach(function(x){var o=el('option','',x[1]);o.value=x[0];if((c.onTranscodeFail||'fail')===x[0])o.selected=true;pf.appendChild(o)});pw.appendChild(pf);g.appendChild(pw);f.onFail=pf;
      card.appendChild(g);var tt=el('button','','test arr connection');var tmsg=el('span','msg');tmsg.style.marginLeft='12px';
      tt.onclick=function(){tmsg.className='msg';tmsg.textContent='testing…';api('POST','/arrs/test',{name:f.name.value,origName:f.orig,kind:f.kind.value,url:f.arrUrl.value,apiKey:f.arrApiKey.value}).then(function(r){tmsg.className='msg ok';tmsg.textContent='connected: '+r.app+' '+r.version}).catch(function(e){tmsg.className='msg err';tmsg.textContent=e.message})};
      card.appendChild(tt);card.appendChild(tmsg);
      var sw=el('button','','place files already in this category\u2019s folder');sw.style.marginLeft='12px';sw.title='uses the saved library folder; save first if you just changed it';var smsg=el('span','msg');smsg.style.marginLeft='12px';
      sw.onclick=function(){smsg.className='msg';smsg.textContent='placing…';api('POST','/library/sweep',{name:f.orig||f.name.value}).then(function(r){smsg.className='msg ok';smsg.textContent=r.result}).catch(function(e){smsg.className='msg err';smsg.textContent=e.message})};
      if(c.libraryDir){card.appendChild(sw);card.appendChild(smsg)}card.appendChild(document.createElement('br'));
      var rm=el('button','bad','remove category');rm.onclick=function(){card.remove()};card.appendChild(rm);card._f=f;cats.appendChild(card)
    }
    S.categories.forEach(add);
    var a=el('button','','+ add category');a.onclick=function(){add({kind:'radarr',transcode:true})};body.appendChild(a);
    saveBar(body,function(m){save(function(b){b.categories=[].map.call(cats.children,function(c){var f=c._f;return {name:f.name.value,origName:f.orig,kind:f.kind.value,libraryDir:f.libraryDir.value,arrUrl:f.arrUrl.value,arrApiKey:f.arrApiKey.value,transcode:f.transcode.checked,onTranscodeFail:f.onFail.value}})},m)})
  }

  function clientsPane(body){
    body.appendChild(head('Download clients','Overmynd downloads through TorBox when a token is set. Otherwise usenet goes to NZBGet and torrents go to qBittorrent, both optional and local.'));
    var tb=el('div','card');var h=el('h4','','TorBox ');h.appendChild(chip(S.torbox.hasToken,S.torbox.hasToken?'token set':'no token'));tb.appendChild(h);
    var g=el('div','grid3');
    var tok=row(g,'API token',{type:'password',ph:S.torbox.hasToken?'set — leave blank to keep':'paste your token',help:'TorBox → Settings → API'});
    var cph=row(g,'new downloads per hour',{type:'number',value:S.torbox.createPerHour,help:'TorBox’s limit on creating downloads'});
    var pa=row(g,'files copied at once',{type:'number',value:S.torbox.pullersAtOnce,help:'how many finished files are pulled down in parallel'});
    var lg=row(g,'seconds between link requests',{type:'number',value:S.torbox.linkGapSeconds,help:'spaces out requests so TorBox does not throttle'});
    tb.appendChild(g);body.appendChild(tb);
    var ng=el('div','card');var h2=el('h4','','NZBGet (usenet fallback) ');h2.appendChild(chip(!!S.nzbget.url,S.nzbget.url?'configured':'not set'));ng.appendChild(h2);
    var g2=el('div','grid3');
    var nu=row(g2,'address',{value:S.nzbget.url,ph:'http://10.0.0.6:6789'});var nn=row(g2,'username',{value:S.nzbget.username});
    var np=row(g2,'password',{type:'password',ph:S.nzbget.hasPassword?'set — leave blank to keep':''});
    var nd=row(g2,'destination folder',{value:S.nzbget.destRoot,help:'NZBGet’s DestDir, at the same path in both containers'});
    ng.appendChild(g2);body.appendChild(ng);
    var qb=el('div','card');var h3=el('h4','','qBittorrent (torrent fallback) ');h3.appendChild(chip(!!S.qbit.url,S.qbit.url?'configured':'not set'));qb.appendChild(h3);
    var g3=el('div','grid3');
    var qu=row(g3,'address',{value:S.qbit.url,ph:'http://10.0.0.7:8080'});var qn=row(g3,'username',{value:S.qbit.username});
    var qp=row(g3,'password',{type:'password',ph:S.qbit.hasPassword?'set — leave blank to keep':''});
    var qd=row(g3,'save folder',{value:S.qbit.saveDir,help:'same path in both containers'});
    qb.appendChild(g3);body.appendChild(qb);
    saveBar(body,function(m){save(function(b){
      b.torbox={token:tok.value,createPerHour:+cph.value,pullersAtOnce:+pa.value,linkGapSeconds:+lg.value};
      b.nzbget={url:nu.value,username:nn.value,password:np.value,destRoot:nd.value};
      b.qbit={url:qu.value,username:qn.value,password:qp.value,saveDir:qd.value}},m).then(function(){pane(body)})})
  }

  function transcodePane(body){
    body.appendChild(head('Transcode (Tdarr)','Videos for categories with transcode on are handed to a dedicated Tdarr library. Overmynd watches Tdarr over its API, so it can show progress and notice failures.'));
    var c0=el('div','card');var h0=el('h4','','Connection ');h0.appendChild(chip(!!S.tdarr.url,S.tdarr.url?'configured':'not set — folder hand-off only'));c0.appendChild(h0);
    var g0=el('div','grid3');
    var url=row(g0,'Tdarr address',{value:S.tdarr.url,ph:'http://10.0.0.8:8265',help:'the Tdarr server, port 8265 by default'});
    var key=row(g0,'API key',{type:'password',ph:S.tdarr.hasApiKey?'set — leave blank to keep':'only if Tdarr has authentication on',help:'Tdarr → Tools → API Keys'});
    var lib=row(g0,'library ID',{value:S.tdarr.libraryId,help:'the ID of the dedicated Overmynd library (shown in the library’s settings)'});
    c0.appendChild(g0);var tb=el('button','','test connection');var tm=el('span','msg');tm.style.marginLeft='12px';
    tb.onclick=function(){tm.className='msg';tm.textContent='testing…';api('POST','/tdarr/test',{url:url.value,apiKey:key.value}).then(function(r){tm.className='msg ok';tm.textContent='connected: '+r.nodes+' node(s), '+r.busyWorkers+' busy worker(s)'}).catch(function(e){tm.className='msg err';tm.textContent=e.message})};
    c0.appendChild(tb);c0.appendChild(tm);body.appendChild(c0);
    var c=el('div','card');c.appendChild(el('h4','','Folders and limits'));var g=el('div','grid3');
    var inn=row(g,'input folder (library source)',{value:S.tdarr.inDir,help:'the same path inside Overmynd and Tdarr'});
    var out=row(g,'output folder (library output)',{value:S.tdarr.outDir});
    var mx=row(g,'jobs with Tdarr at once',{type:'number',value:S.tdarr.maxInFlight,help:'others wait their turn; waiting is not a failure'});
    var st=row(g,'stall timeout (minutes)',{type:'number',value:S.tdarr.stallMinutes,help:'a worker holding a file with no progress this long counts as failed'});
    var to=row(g,'give up waiting after (minutes)',{type:'number',value:S.tdarr.timeoutMinutes,help:'when Tdarr produces nothing at all'});
    c.appendChild(g);body.appendChild(c);
    var cc=el('div','card');cc.appendChild(el('h4','','Per category'));
    S.categories.forEach(function(k){var d=el('div','kv');d.appendChild(el('span','k',k.name));d.appendChild(chip(k.transcode,k.transcode?'transcode on':'skips Tdarr'));if(k.transcode)d.appendChild(el('span','v','on failure: '+(k.onTranscodeFail==='passthrough'?'keep the original':'fail the grab')));cc.appendChild(d)});
    cc.appendChild(help('Change transcode and the failure policy on the Arrs & categories tab.'));body.appendChild(cc);
    saveBar(body,function(m){save(function(b){b.tdarr={url:url.value,apiKey:key.value,libraryId:lib.value,maxInFlight:+mx.value,stallMinutes:+st.value,inDir:inn.value,outDir:out.value,timeoutMinutes:+to.value}},m)})
  }

  function playbackPane(body){
    var P=S.playback||{};
    body.appendChild(head('Playback (Tracearr and Emby)','Feeds the public page: Now Playing and Recently Added come from Tracearr (music is added from Lidarr). Emby is only used to hide whole libraries from Now Playing.'));
    var c=el('div','card');var h=el('h4','','Tracearr ');h.appendChild(chip(!!P.tracearrUrl&&P.hasTracearrKey,P.tracearrUrl&&P.hasTracearrKey?'configured':'not set'));c.appendChild(h);
    var g=el('div','grid3');
    var tu=row(g,'Tracearr address',{value:P.tracearrUrl,ph:'http://10.0.0.8:3000'});
    var tk=row(g,'Public API key',{type:'password',ph:P.hasTracearrKey?'set — leave blank to keep':'Tracearr → Settings → Public API',help:'read-only access to streams and recently added'});
    c.appendChild(g);body.appendChild(c);
    var c2=el('div','card');var h2=el('h4','','Hidden libraries ');h2.appendChild(chip(!!P.hiddenLibraries,P.hiddenLibraries?'hiding '+P.hiddenLibraries:'none hidden'));c2.appendChild(h2);
    var g2=el('div','grid3');
    var eu=row(g2,'Emby address',{value:P.embyUrl,ph:'http://10.0.0.8:8096'});
    var ek=row(g2,'Emby API key',{type:'password',ph:P.hasEmbyKey?'set — leave blank to keep':'Emby → Dashboard → API Keys'});
    var hl=row(g2,'hidden library IDs',{value:P.hiddenLibraries,ph:'9',help:'comma-separated Emby library IDs; streams from these never appear on the public page. A stream that cannot be placed in a library is withheld.'});
    c2.appendChild(g2);
    var tb=el('button','','test connections');var tm=el('span','msg');tm.style.marginLeft='12px';
    tb.onclick=function(){tm.className='msg';tm.textContent='testing…';api('POST','/playback/test',{tracearrUrl:tu.value,tracearrKey:tk.value,embyUrl:eu.value,embyKey:ek.value}).then(function(r){tm.className='msg ok';tm.textContent=[r.tracearr?'Tracearr ok':'',r.emby?'Emby ok: '+r.emby:''].filter(Boolean).join(' · ')||'nothing to test'}).catch(function(e){tm.className='msg err';tm.textContent=e.message})};
    c2.appendChild(tb);c2.appendChild(tm);body.appendChild(c2);
    var ps=el('p','hint','last poll: checking…');body.appendChild(ps);api('GET','/playback/status').then(function(r){ps.textContent='last poll: '+r.status}).catch(function(){ps.textContent='last poll: unknown'});
    saveBar(body,function(m){save(function(b){b.playback={tracearrUrl:tu.value,tracearrKey:tk.value,embyUrl:eu.value,embyKey:ek.value,hiddenLibraries:hl.value}},m).then(function(){pane(body)})})
  }

  function publicPane(body){
    var D=S.donations||{};
    body.appendChild(head('Public page','What visitors see on the public status page.'));
    var c=el('div','card');var h=el('h4','','Donate button ');h.appendChild(chip(!!(D.paypal_url||D.cashapp_url||D.custom_url),(D.paypal_url||D.cashapp_url||D.custom_url)?'shown':'hidden — no links'));c.appendChild(h);
    var g=el('div','grid3');
    var ti=row(g,'popover title',{value:D.title,ph:'Support this server'});
    var pp=row(g,'PayPal link',{value:D.paypal_url,ph:'https://paypal.me/…'});
    var ca=row(g,'Cash App link',{value:D.cashapp_url,ph:'https://cash.app/$…'});
    var cl=row(g,'other link label',{value:D.custom_label,ph:'Ko-fi'});
    var cu=row(g,'other link URL',{value:D.custom_url,ph:'https://'});
    c.appendChild(g);c.appendChild(help('Links are public. Leave them all blank to hide the Donate button. HTTPS only.'));body.appendChild(c);
    var R=S.requests||{};
    var c2=el('div','card');var h2=el('h4','','Media requests (Seerr) ');h2.appendChild(chip(!!R.enabled,R.enabled?'on':'off'));c2.appendChild(h2);
    var g2=el('div','grid3');
    var re=row(g2,'let visitors request media',{type:'checkbox',value:R.enabled,help:'shows the Request media box on the public page'});
    var ru=row(g2,'Seerr address',{value:R.url,ph:'http://10.0.0.8:5055'});
    var rk=row(g2,'Seerr API key',{type:'password',ph:R.hasApiKey?'set — leave blank to keep':'Seerr → Settings → General',help:'never shown on the public page'});
    var ri=row(g2,'request as user ID',{type:'number',value:R.userId||'',help:'the Seerr user visitors request as; Seerr applies that user’s permissions, quota and auto-approval'});
    c2.appendChild(g2);var rb=el('button','','test');var rm=el('span','msg');rm.style.marginLeft='12px';
    rb.onclick=function(){rm.className='msg';rm.textContent='testing…';api('POST','/requests/test',{url:ru.value,apiKey:rk.value,userId:+ri.value}).then(function(){rm.className='msg ok';rm.textContent='connected, and the key can act as that user'}).catch(function(e){rm.className='msg err';rm.textContent=e.message})};
    c2.appendChild(rb);c2.appendChild(rm);body.appendChild(c2);
    var c3=el('div','card');c3.appendChild(el('h4','','Awaiting approval'));var lst=el('div');c3.appendChild(lst);body.appendChild(c3);
    function loadPending(){lst.replaceChildren();api('GET','/requests').then(function(r){if(!r.requests.length){lst.appendChild(help('Nothing is waiting.'));return}
      r.requests.forEach(function(q){var d=el('div','kv');d.appendChild(el('span','k',(q.title||'Untitled')+' ('+(q.media_type==='tv'?'TV show':'Movie')+')'));d.appendChild(el('span','v',q.requested_by||''));
        [['approve','approve'],['decline','deny']].forEach(function(a){var b=el('button','',a[1]);b.onclick=function(){b.disabled=true;api('POST','/requests/'+q.source_id+'/'+a[0]).then(loadPending).catch(function(e){b.disabled=false;alert(e.message)})};d.appendChild(b)});lst.appendChild(d)})}).catch(function(){lst.appendChild(help('Seerr is not reachable or not set up yet.'))})}
    loadPending();
    accessCard(body);
    saveBar(body,function(m){save(function(b){b.requests={enabled:re.checked,url:ru.value,apiKey:rk.value,userId:+ri.value};b.donations={title:ti.value,paypal_url:pp.value,cashapp_url:ca.value,custom_label:cl.value,custom_url:cu.value}},m).then(function(){pane(body)})})
  }

  function accessCard(body){
    var c=el('div','card');var h=el('h4','','Account requests ("need access?") ');var hc=el('span');h.appendChild(hc);c.appendChild(h);
    c.appendChild(help('Visitors ask for a media server account. Approving creates a disabled copy of your template user, and the visitor finishes setup from an emailed single-use link. The media server address and key come from the Playback tab. Emails are sent over TLS/STARTTLS only.'));
    var g=el('div','grid3'),A={};
    var en=row(g,'let visitors request accounts',{type:'checkbox'});
    var pv=row(g,'server type',{help:'emby or jellyfin'});
    var pu=row(g,'this site’s public HTTPS address',{ph:'https://status.example.com',help:'used in the emailed links'});
    var sp=row(g,'media server web address',{ph:'https://emby.example.com',help:'HTTPS. Shown in the “you’re ready to watch” email so people know where to watch. Required for Jellyfin.'});
    var tp=row(g,'template user ID',{help:'a disabled, non-admin user whose permissions new accounts copy'});
    var sh=row(g,'SMTP host',{ph:'smtp.example.com'});
    var sn=row(g,'SMTP port',{type:'number'});
    var ss=row(g,'SMTP security',{help:'tls (port 465) or starttls (587)'});
    var su=row(g,'SMTP username');
    var sw=row(g,'SMTP password',{type:'password'});
    var fr=row(g,'send from',{ph:'undermynd@example.com'});
    var ad=row(g,'admin email',{ph:'you@example.com',help:'told about new requests and problems'});
    c.appendChild(g);
    var msg=el('span','msg');msg.style.marginLeft='12px';
    var sv=el('button','go','save access settings'),tb=el('button','','send test email');
    sv.onclick=function(){msg.className='msg';msg.textContent='saving…';
      api('PUT','/access',{enabled:en.checked,provider:pv.value.trim().toLowerCase(),public_url:pu.value,server_public_url:sp.value,template_id:tp.value.trim(),smtp_host:sh.value.trim(),smtp_port:+sn.value,smtp_security:ss.value.trim().toLowerCase(),smtp_user:su.value,smtp_password:sw.value,from:fr.value.trim(),admin_email:ad.value.trim()})
        .then(function(v){sw.value='';fill(v);msg.className='msg ok';msg.textContent='saved'}).catch(function(e){msg.className='msg err';msg.textContent=e.message})};
    tb.onclick=function(){msg.className='msg';msg.textContent='sending…';api('POST','/access/test-email',{}).then(function(){msg.className='msg ok';msg.textContent='test email sent to the admin address'}).catch(function(e){msg.className='msg err';msg.textContent=e.message})};
    c.appendChild(sv);tb.style.marginLeft='8px';c.appendChild(tb);c.appendChild(msg);
    function fill(v){en.checked=!!v.enabled;pv.value=v.provider||'emby';pu.value=v.public_url||'';sp.value=v.server_public_url||'';tp.value=v.template_id||'';sh.value=v.smtp_host||'';sn.value=v.smtp_port||'';ss.value=v.smtp_security||'';su.value=v.smtp_user||'';fr.value=v.from||'';ad.value=v.admin_email||'';sw.placeholder=v.has_smtp_password?'set — leave blank to keep':'';hc.replaceChildren(chip(!!v.enabled,v.enabled?'on':'off'))}
    body.appendChild(c);
    var q=el('div','card');q.appendChild(el('h4','','Account request queue'));var lst=el('div');q.appendChild(lst);body.appendChild(q);
    var names={pending:'awaiting approval',incomplete:'setup needs attention',provisioning:'creating account',awaiting_setup:'waiting for the visitor to finish setup',setting_up:'finishing setup',active:'active',declined:'declined'};
    function act(id,a,b,note){b.disabled=true;api('POST','/access/requests/'+id+'/'+a,note||{}).then(loadQ).catch(function(e){b.disabled=false;alert(e.message)})}
    function loadQ(){api('GET','/access/requests').then(function(r){lst.replaceChildren();var items=r.requests||[];if(!items.length){lst.appendChild(help('No account requests yet.'));return}
      items.forEach(function(it){var d=el('div','kv');d.appendChild(el('span','k',it.name+' — '+(names[it.status]||it.status)));d.appendChild(el('span','v',it.email+' · '+it.username+(it.connect_username?' · Connect email '+it.connect_username:'')));
        [it.error,it.mail_error].forEach(function(t){if(t){var e=el('div','msg err',t);d.appendChild(e)}});
        var acts=[];
        if(it.status==='pending')acts=[['approve','approve'],['decline','decline'],['resend','resend notices']];
        else if(it.status==='incomplete')acts=[['approve','retry setup']];
        else if(it.status==='awaiting_setup')acts=[['resend','send fresh setup link']];
        else if(it.status==='active')acts=[['resend','resend sign-in instructions']];
        else if(it.status==='declined')acts=[['resend','resend notice']];
        if(it.emby_id&&['incomplete','awaiting_setup','active'].indexOf(it.status)>=0)acts.push(['reapply-template','reapply template'],['reset','reset after server deletion']);
        acts.forEach(function(a){var b=el('button','',a[1]);b.onclick=function(){act(it.id,a[0],b)};d.appendChild(b)});
        if(it.status==='pending'||it.status==='incomplete'){var b2=el('button','','correct details');b2.onclick=function(){var em=prompt_('Notification email',it.email);if(em==null)return;var cn=it.connect_username?prompt_('Emby Connect email',it.connect_username):'';if(cn==null)return;b2.disabled=true;api('POST','/access/requests/'+it.id+'/correct',{email:em,connect_username:cn}).then(loadQ).catch(function(e){b2.disabled=false;alert(e.message)})};d.appendChild(b2)}
        lst.appendChild(d)})}).catch(function(e){lst.replaceChildren(help(e.message))})}
    function prompt_(l,v){return window.prompt(l,v)}
    api('GET','/access').then(fill).catch(function(e){msg.className='msg err';msg.textContent=e.message});loadQ();
  }

  function rssPane(body){
    body.appendChild(head('RSS auto-download','Feeds are checked on a schedule. The first check of a new feed only records what is already there, so nothing old gets downloaded. The first rule that matches an item starts it.'));
    body.appendChild(el('div','sec','what the feeds are reading'));
    var live=el('div');body.appendChild(live);
    var cats=[];api('GET','/settings').then(function(v){cats=(v.categories||[]).map(function(c){return c.name})}).catch(function(){});
    // "download" turns into a category picker; starting it goes through the same path as a rule.
    function manual(feed,idx,title,cell){var btn=el('button','','download');btn.onclick=function(){
      var sel=document.createElement('select');cats.forEach(function(n){var o=document.createElement('option');o.textContent=n;sel.appendChild(o)});
      var go=el('button','go','start'),no=el('button','','cancel'),msg=el('span','msg');
      function back(){cell.replaceChildren(manual(feed,idx,title,cell))}
      no.onclick=back;
      go.onclick=function(){go.disabled=true;msg.textContent='starting…';api('POST','/rss/download',{feed:feed,index:idx,title:title,category:sel.value}).then(function(){showFeeds()}).catch(function(e){go.disabled=false;msg.className='msg err';msg.textContent=e.message})};
      [sel,go,no,msg].forEach(function(x){x.style.marginRight='6px';cell.appendChild(x)});btn.remove()};return btn}
    function showFeeds(){api('GET','/rss/status').then(function(list){live.textContent='';if(!list.length)live.appendChild(help('No feeds yet.'));list.forEach(function(st){
      var card=el('div','card');var h=el('h4','',st.name+'  ');var when=st.checkedAt&&st.checkedAt.indexOf('0001')!==0?'checked '+new Date(st.checkedAt).toLocaleTimeString():'not checked yet';h.appendChild(el('span','',when));card.appendChild(h);
      if(st.error){var er=el('div','msg err',st.error);card.appendChild(er)}
      var t=document.createElement('table');(st.items||[]).forEach(function(it,idx){var tr=document.createElement('tr');var a=document.createElement('td');a.textContent=it.title;var b=document.createElement('td');b.textContent=it.outcome;b.style.opacity='.75';b.style.paddingLeft='16px';b.style.whiteSpace='nowrap';tr.appendChild(a);tr.appendChild(b);var c=document.createElement('td');c.style.whiteSpace='nowrap';c.style.paddingLeft='12px';if(it.outcome.indexOf('started')!==0)c.appendChild(manual(st.name,idx,it.title,c));tr.appendChild(c);t.appendChild(tr)});
      if(st.items&&st.items.length)card.appendChild(t);else if(!st.error)card.appendChild(help('Nothing read from this feed yet.'));
      live.appendChild(card)})}).catch(function(e){live.textContent=e.message})}
    var chk=el('button','','check feeds now');chk.onclick=function(){chk.textContent='checking…';api('POST','/rss/check',{}).then(function(){chk.textContent='check feeds now';showFeeds()}).catch(function(){chk.textContent='check feeds now'})};body.appendChild(chk);
    showFeeds();
    body.appendChild(el('div','sec','feeds'));
    var feeds=el('div');body.appendChild(feeds);
    function addFeed(c){var card=el('div','card');var g=el('div','grid3');var f={};
      f.name=row(g,'name',{value:c.name});f.url=row(g,'feed address',{value:c.url,help:'the full RSS link, including any key'});
      f.intervalMinutes=row(g,'check every (minutes)',{type:'number',value:c.intervalMinutes||15});f.enabled=row(g,'enabled',{type:'checkbox',value:c.enabled!==false});
      card.appendChild(g);var rm=el('button','bad','remove feed');rm.onclick=function(){card.remove()};card.appendChild(rm);card._f=f;feeds.appendChild(card)}
    S.feeds.forEach(addFeed);var af=el('button','','+ add feed');af.onclick=function(){addFeed({enabled:true,intervalMinutes:15})};body.appendChild(af);
    body.appendChild(el('div','sec','rules'));
    body.appendChild(help('Patterns are regular expressions and ignore case. Example: must contain  S\\d+E\\d+.*1080p   must not contain  cam|hdts'));
    var rules=el('div');body.appendChild(rules);
    function addRule(c){var card=el('div','card');var g=el('div','grid3');var f={};
      f.name=row(g,'rule name',{value:c.name});f.feed=row(g,'only this feed',{value:c.feed,help:'blank = every feed'});
      f.category=row(g,'category',{value:c.category,help:'one of your categories'});
      f.mustContain=row(g,'must contain',{value:c.mustContain});f.mustNotContain=row(g,'must not contain',{value:c.mustNotContain});
      f.enabled=row(g,'enabled',{type:'checkbox',value:c.enabled!==false});
      card.appendChild(g);var rm=el('button','bad','remove rule');rm.onclick=function(){card.remove()};card.appendChild(rm);card._f=f;rules.appendChild(card)}
    S.rules.forEach(addRule);var ar=el('button','','+ add rule');ar.onclick=function(){addRule({enabled:true})};body.appendChild(ar);
    body.appendChild(el('div','sec','watched folders'));
    body.appendChild(help('Drop a .torrent, .magnet or .nzb file in a folder and it goes through the same flow. Handled files move to a submitted-to-undermynd subfolder. Finished files are kept in the Complete folder under the category. Do not point this at a folder another app also watches.'));
    var watches=el('div');body.appendChild(watches);
    function addWatch(c){var card=el('div','card');var g=el('div','grid3');var f={};
      f.dir=row(g,'folder',{value:c.dir,help:'full path as the container sees it'});
      f.category=row(g,'category',{value:c.category,help:'blank = the category of your first enabled RSS rule'});
      f.enabled=row(g,'enabled',{type:'checkbox',value:c.enabled!==false});
      card.appendChild(g);var rm=el('button','bad','remove folder');rm.onclick=function(){card.remove()};card.appendChild(rm);card._f=f;watches.appendChild(card)}
    (S.watches||[]).forEach(addWatch);var aw=el('button','','+ add folder');aw.onclick=function(){addWatch({enabled:true})};body.appendChild(aw);
    saveBar(body,function(m){save(function(b){
      b.watches=[].map.call(watches.children,function(c){var f=c._f;return {dir:f.dir.value,category:f.category.value,enabled:f.enabled.checked}});
      b.feeds=[].map.call(feeds.children,function(c){var f=c._f;return {name:f.name.value,url:f.url.value,intervalMinutes:+f.intervalMinutes.value,enabled:f.enabled.checked}});
      b.rules=[].map.call(rules.children,function(c){var f=c._f;return {name:f.name.value,feed:f.feed.value,mustContain:f.mustContain.value,mustNotContain:f.mustNotContain.value,category:f.category.value,enabled:f.enabled.checked}})},m)})
  }

  function systemPane(body){
    body.appendChild(head('System','What the pipeline is doing right now, the login the arrs use, and timing.'));
    var libErr=el('div');body.appendChild(libErr);
    api('GET','/library').then(function(d){if(d.errors&&d.errors.length){var c=el('div','card');c.appendChild(el('h4','','Library status problems'));d.errors.slice(0,10).forEach(function(e){c.appendChild(el('div','msg err',e))});libErr.appendChild(c)}}).catch(function(){});
    var live=el('div');body.appendChild(live);
    function bytes(n){if(!n)return '0 B';var u=['B','KB','MB','GB','TB'],i=0;while(n>=1024&&i<4){n/=1024;i++}return n.toFixed(i?1:0)+' '+u[i]}
    function drawSystem(){api('GET','/system').then(function(d){
      live.replaceChildren();var g=el('div','grid3');
      var c1=el('div','card');c1.appendChild(el('h4','','Intake'));var k=el('div','kv');k.appendChild(el('span','k','state'));k.appendChild(chip(!d.paused,d.paused?'paused':'running'));c1.appendChild(k);
      ['queued','downloading','processing','finishing','failed'].forEach(function(x){var r=el('div','kv');r.appendChild(el('span','k',x));r.appendChild(el('span','v',String(d.counts[x]||0)));c1.appendChild(r)});g.appendChild(c1);
      var c2=el('div','card');c2.appendChild(el('h4','','TorBox'));
      if(!d.torbox.configured){c2.appendChild(help('No token set.'))}else{
        var r1=el('div','kv');r1.appendChild(el('span','k','new downloads this hour'));r1.appendChild(el('span','v',d.torbox.used+' of '+d.torbox.limit));c2.appendChild(r1);
        var br=el('div','bar');var bf=el('i');bf.style.width=Math.min(100,d.torbox.limit?d.torbox.used*100/d.torbox.limit:0)+'%';br.appendChild(bf);c2.appendChild(br);
        if(d.torbox.backoffUntil){var r2=el('div','kv');r2.appendChild(el('span','k','backing off until'));r2.appendChild(el('span','v flag',new Date(d.torbox.backoffUntil).toLocaleTimeString()));c2.appendChild(r2)}}
      g.appendChild(c2);
      var c3=el('div','card');c3.appendChild(el('h4','','Tdarr'));var t1=el('div','kv');t1.appendChild(el('span','k','jobs with Tdarr'));t1.appendChild(el('span','v',d.tdarr.inFlight+' of '+d.tdarr.max));c3.appendChild(t1);
      var t2=el('div','kv');t2.appendChild(el('span','k','watched over the API'));t2.appendChild(chip(d.tdarr.api,d.tdarr.api?'yes':'no — folders only'));c3.appendChild(t2);g.appendChild(c3);
      [['Work folder',d.work],['Complete folder',d.complete]].forEach(function(p){var c=el('div','card');c.appendChild(el('h4','',p[0]));var a=el('div','kv');a.appendChild(el('span','k','path'));a.appendChild(el('span','v',p[1].path));c.appendChild(a);
        var b=el('div','kv');b.appendChild(el('span','k','used by Overmynd'));b.appendChild(el('span','v',bytes(p[1].usedByUs)+(p[1].partial?' (partial count)':'')));c.appendChild(b);
        if(p[1].totalBytes){var f=el('div','kv');f.appendChild(el('span','k','free on this disk'));f.appendChild(el('span','v',bytes(p[1].freeBytes)+' of '+bytes(p[1].totalBytes)));c.appendChild(f)}g.appendChild(c)});
      live.appendChild(g)}).catch(fail)}
    drawSystem();timer=setInterval(function(){if(tab==='system')drawSystem()},5000);
    var c=el('div','card');c.appendChild(el('h4','','Client login for the arrs'));
    c.appendChild(help('Used as the qBittorrent login, and the password is also the SABnzbd API key. It is never shown again; generate a new one to see it once.'));
    var u=el('div','kv');u.appendChild(el('span','k','username'));u.appendChild(el('span','v',S.clientUser));c.appendChild(u);
    var out=el('pre');var gen=el('button','','generate new password');
    gen.onclick=function(){if(!confirm('This replaces the current password. The arrs must be updated.'))return;api('POST','/settings/client-password').then(function(r){out.textContent='username: '+r.user+'\npassword: '+r.password+'\n(shown once)'}).catch(fail)};
    c.appendChild(gen);c.appendChild(out);body.appendChild(c);
    var t=el('div','card');t.appendChild(el('h4','','Timing'));var g=el('div','grid3');
    var st=row(g,'no-progress timeout (minutes)',{type:'number',value:S.stallMinutes,help:'a download with no progress this long is retried'});
    var rc=row(g,'import check interval (seconds)',{type:'number',value:S.reconcileSeconds,help:'how often the arr is asked whether it imported'});
    var fl=row(g,'flag a ready job after (hours)',{type:'number',value:S.flagImportHours,help:'a job the arr has not imported for this long is flagged'});
    var sc=row(g,'storage scan interval (minutes)',{type:'number',value:S.storageScanMinutes,help:'how often library sizes are measured in the background; the last result is saved and shown right away after a restart'});
    var qc=row(g,'check resolution against the arr',{type:'checkbox',value:S.qualityCheck!==false,help:'a download whose real video size is below the quality the arr grabbed (a 1080p release that is really 480p) fails, so the arr blocklists it and searches again'});
    t.appendChild(g);body.appendChild(t);
    saveBar(body,function(m){save(function(b){b.qualityCheck=qc.checked;b.stallMinutes=+st.value;b.reconcileSeconds=+rc.value;b.flagImportHours=+fl.value;b.storageScanMinutes=+sc.value},m)})
  }
  start();
})();
