import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';
const root=process.cwd(),state=mkdtempSync(join(tmpdir(),'mace-live-'));
const env={...process.env,MACE_HOME:state,MACE_AUTH_SECRET:'',MACE_TEST_MODE:'',MACE_PID_FILE:join(state,'maced.pid'),MACE_LOG_FILE:join(state,'maced.log'),MACE_FILE_ROOT:root};
const binary=join(root,'bin/mace');let browser;
try{
 const start=spawnSync(binary,['start','--host','127.0.0.1','--port','0','--db',join(state,'mace.db')],{env,encoding:'utf8'});assert.equal(start.status,0,start.stderr);
 const runtime=JSON.parse(readFileSync(join(state,'runtime.json'),'utf8')),token=readFileSync(join(state,'active-token'),'utf8');
 async function api(path,body){const r=await fetch(runtime.url+'/api/v1'+path,{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});if(!r.ok)throw Error(r.status+' '+await r.text());return r.json()}
 async function wait(room){const deadline=Date.now()+180000;let printed=0;while(Date.now()<deadline){const run=(await api('/runs')).find(r=>r.id===room);if(run&&run.status!=='running'){if(run.error)throw Error(run.error);return run}if(Date.now()-printed>20000){console.log('Waiting for live CLI reply…');printed=Date.now()}await new Promise(r=>setTimeout(r,500))}await api('/chats/'+room+'/pause',{});throw Error('live reply exceeded 180 seconds; cancelled')}
 const catalog=await api('/catalog');for(const p of ['agy','muse'])assert(catalog.providers.some(a=>a.id===p&&a.available),p+' not discovered');
 const sessions=[];
 for(const provider of ['agy','muse']){
  const s=await api('/sessions',{provider,working_dir:root});sessions.push(s);const marker='MACE_'+provider.toUpperCase()+'_LIVE_OK';
  await api('/chats/'+s.id+'/messages',{text:'Reply exactly '+marker+'. Do not use tools. Do not read, create, or modify any files.'});
  const run=await wait(s.id),messages=await api('/chats/'+s.id+'/messages');const reply=messages.filter(m=>m.sender===s.id).at(-1)?.text;
  assert(reply?.includes(marker),provider+' did not return expected marker: '+reply);console.log('LIVE PASS',provider,run.status,JSON.stringify(reply));
 }
 const group=await api('/rooms',{name:'Live agent collaboration',members:sessions.map(s=>s.id)});
 await api('/chats/'+group.id+'/messages',{text:'Each participant: reply with your agent name followed by MACE_GROUP_OK only. Do not use tools. Do not read, create, or modify files.'});
 const run=await wait(group.id),messages=await api('/chats/'+group.id+'/messages');assert.equal(run.replies,2);assert.equal(messages.filter(m=>m.sender!=='you'&&m.text.includes('MACE_GROUP_OK')).length,2);console.log('LIVE PASS agy + muse group, 2 native replies');
 if(process.env.MACE_PLAYWRIGHT){const{chromium}=await import(process.env.MACE_PLAYWRIGHT);browser=await chromium.launch({executablePath:process.env.MACE_BROWSER,headless:true,args:['--no-sandbox']});const page=await browser.newPage({viewport:{width:1360,height:900}});await page.goto(runtime.url+'/#connect='+token);await page.getByRole('button',{name:/Live agent collaboration/}).click();await page.waitForFunction(()=>document.querySelectorAll('.message-text').length===3);await page.screenshot({path:'/tmp/mace-live-agents.png'});console.log('LIVE PASS browser rendered both real agent replies')}
 console.log('Live test state and logs:',state);
}finally{if(browser)await browser.close();const stopped=spawnSync(binary,['stop'],{env,encoding:'utf8',timeout:15000});console.log(stopped.stdout.trim());if(stopped.status!==0)console.error(stopped.stderr)}
