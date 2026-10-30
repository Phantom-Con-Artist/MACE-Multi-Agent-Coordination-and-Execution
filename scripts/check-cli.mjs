import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';
import net from 'node:net';
import http from 'node:http';
const root=process.cwd(),state=mkdtempSync(join(tmpdir(),'mace-cli-')),binary=process.argv[2]||join(root,'bin/mace');
const env={...process.env,MACE_HOME:state,MACE_AUTH_SECRET:'',MACE_TEST_MODE:'1',MACE_PID_FILE:join(state,'maced.pid'),MACE_LOG_FILE:join(state,'maced.log')};
function cli(args,extra={}){return spawnSync(binary,args,{cwd:tmpdir(),env:{...env,...extra},encoding:'utf8',timeout:30000})}
const info=()=>JSON.parse(readFileSync(join(state,'runtime.json'),'utf8'));
const token=()=>readFileSync(join(state,'active-token'),'utf8');
function dead(pid){try{const stat=readFileSync('/proc/'+pid+'/stat','utf8');return stat.slice(stat.lastIndexOf(')')+2).startsWith('Z')}catch{return true}}
async function api(path,body){return new Promise((resolve,reject)=>{const req=http.request(info().url+'/api/v1'+path,{agent:false,method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token(),'Content-Type':'application/json'}},res=>{let data='';res.on('data',b=>data+=b);res.on('end',()=>{if(res.statusCode!==200&&res.statusCode!==201){reject(Error(path+' returned '+res.statusCode+': '+data));return}try{resolve(JSON.parse(data))}catch(e){reject(e)}})});req.on('error',e=>reject(Error(path+': '+e.message)));if(body!==undefined)req.write(JSON.stringify(body));req.end()})}
let occupied,stoppedPID;
try{
 assert(cli(['help']).stdout.includes('doctor'));assert(cli(['version']).stdout.includes('MACE 0.1.0'));assert(cli(['version']).stdout.includes('Multi Agent Coordination and Execution'));
 for(const args of [['start','--port','no'],['start','--port','-1'],['start','--port','65536'],['start','--host']])assert.notEqual(cli(args).status,0);
 occupied=net.createServer();await new Promise(r=>occupied.listen(0,'127.0.0.1',r));const used=occupied.address().port;
 const conflict=cli(['start','--host','127.0.0.1','--port',String(used)]);assert.notEqual(conflict.status,0);assert(conflict.stderr.includes('cannot bind'));assert(!existsSync(env.MACE_PID_FILE));assert(cli(['doctor','--host','127.0.0.1','--port',String(used)]).stdout.includes('occupied'));
 const refusal=cli(['stop','--port',String(used)]);assert.notEqual(refusal.status,0);assert(refusal.stderr.includes('no unrelated process was stopped'));assert(occupied.listening);
 await new Promise(r=>occupied.close(r));occupied=undefined;
 // A stale lock file and a PID belonging to this Node test must never block or kill it.
 writeFileSync(env.MACE_PID_FILE,String(process.pid));writeFileSync(env.MACE_PID_FILE+'.lock','stale lock');
 const start=cli(['start','--host','127.0.0.1','--port','0','--db',join(state,'mace.db')]);assert.equal(start.status,0,start.stderr);const first=info();assert(first.port>0);assert(start.stdout.includes('Server ready'));assert.equal(cli(['status']).status,0);assert(cli(['status']).stdout.includes('Health: ready'));
 assert(cli(['ports']).stdout.includes(String(first.port)));assert.equal(cli(['status','--port',String(first.port)]).status,0);
 const homepage=await fetch(first.url,{headers:{Connection:'close'}});assert.equal(homepage.status,200);await homepage.text();assert.equal(cli(['start']).status,0);assert.equal(info().pid,first.pid);assert(cli(['start']).stdout.includes('already running'));
 assert(cli(['info']).stdout.includes(token()));assert.equal(cli(['token']).stdout.trim(),token());assert.equal(cli(['logs','--lines','2']).status,0);assert.notEqual(cli(['logs','--lines','0']).status,0);
 const other=join(state,'other');mkdirSync(other);const otherConflict=cli(['start','--host','127.0.0.1','--port',String(first.port)],{MACE_HOME:other,MACE_PID_FILE:join(other,'pid'),MACE_LOG_FILE:join(other,'log')});assert.notEqual(otherConflict.status,0);assert(otherConflict.stderr.includes('already serves Mace'));
 const agent=await api('/sessions',{provider:'diagnostic'});assert(cli(['agents']).stdout.includes(agent.id));
 process.kill(first.pid,'SIGSTOP');stoppedPID=first.pid;const unhealthy=cli(['status']);assert.notEqual(unhealthy.status,0);assert(unhealthy.stderr.includes('process is alive'));process.kill(first.pid,'SIGCONT');stoppedPID=undefined;
 const savedToken=token();assert.equal(cli(['restart']).status,0);assert.notEqual(info().pid,first.pid);assert.equal(token(),savedToken);assert(cli(['agents']).stdout.includes(agent.id));
 const job=await api('/agents/',{name:'force-stop tree',command:'sh',args:['-c','sleep 300 & wait']});assert(cli(['ps']).stdout.includes('force-stop tree'));await new Promise(r=>setTimeout(r,150));
 const descendants=readFileSync('/proc/'+job.pid+'/task/'+job.pid+'/children','utf8').trim().split(/\s+/).filter(Boolean).map(Number);
 const killedPort=info().port;assert.equal(cli(['kill','--port',String(killedPort)]).status,0);for(let i=0;i<30&&(!dead(job.pid)||descendants.some(p=>!dead(p)));i++)await new Promise(r=>setTimeout(r,100));assert(dead(job.pid),'native process survived forced shutdown');assert(descendants.every(dead),'native child survived forced shutdown');assert.equal(cli(['stop','--port',String(killedPort)]).status,0);
 assert.equal(cli(['stop']).status,0);assert.equal(cli(['kill']).status,0);assert.notEqual(cli(['status']).status,0);
 const missing=cli(['start','--port','0'],{MACE_DAEMON:join(state,'missing-daemon')});assert.notEqual(missing.status,0);assert(missing.stderr.includes('daemon executable unavailable'));
 const failed=cli(['start','--host','127.0.0.1','--port','0','--not-a-real-flag']);assert.notEqual(failed.status,0);assert(failed.stderr.includes('before becoming ready'));assert(!existsSync(env.MACE_PID_FILE));
 console.log('PASS: installed-anywhere start, useful help/version, invalid ports, occupied ports, duplicate starts, stale PID/lock recovery, HTTP health failure, info/token/logs/agents/ps, restart persistence, forced child cleanup, repeated stop/kill, missing daemon, failed startup cleanup.');
}finally{if(stoppedPID)process.kill(stoppedPID,'SIGCONT');if(occupied)occupied.close();cli(['stop'])}
