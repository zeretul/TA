import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { createHash } from 'node:crypto'
import { readFile,writeFile,mkdir,readdir } from 'node:fs/promises'
import path from 'node:path'
const root=path.resolve(import.meta.dirname,'../../..')
const out=path.join(root,'.ta-dev/070-repair')
const installed=JSON.parse(await readFile(path.join(out,'installation.json'),'utf8'))
const port=19928, base=`http://127.0.0.1:${port}`
await new Promise((resolve,reject)=>{const probe=createServer();probe.once('error',reject);probe.listen(port,'127.0.0.1',()=>probe.close(resolve))})
const hash=b=>createHash('sha256').update(b).digest('hex')
async function list(dir){const result=[];for(const e of await readdir(dir,{withFileTypes:true})){const p=path.join(dir,e.name);if(e.isDirectory())result.push(...await list(p));else if(e.isFile())result.push(p);else throw new Error('Unexpected asset link')}return result}
const results=[]
for(const entry of installed.installations){
 const data=path.join(out,'installation-qa',entry.name)
 await mkdir(data,{recursive:true});await writeFile(path.join(data,'config.toml'),'update_check_enabled = false\nlanguage = "zh-CN"\n')
 const processLog=[]
 const child=spawn(path.join(entry.path,'TA.exe'),['--no-open',`--port=${port}`],{cwd:data,windowsHide:true,env:{...process.env,TA_DIR:data,DENOVA_DIR:'',TA_WORKSPACE:'',DENOVA_WORKSPACE:'',TA_WEB_DIR:'',GOMAXPROCS:'4'},stdio:['ignore','pipe','pipe']})
 child.stdout.on('data',b=>processLog.push(b.toString()));child.stderr.on('data',b=>processLog.push(b.toString()))
 try{
  let ready=false
  for(let i=0;i<80;i++){
   try{const r=await fetch(base+'/api/status',{signal:AbortSignal.timeout(1000)});if(r.ok){ready=true;break}}catch{}
   if(child.exitCode!==null)throw new Error('Installed program exited: '+processLog.join(''))
   await new Promise(resolve=>setTimeout(resolve,250))
  }
  if(!ready)throw new Error('Installed program did not become ready')
  const staticRoot=path.join(entry.path,'web')
  const paths=await list(staticRoot)
  let verified=0
  for(let i=0;i<paths.length;i+=6)await Promise.all(paths.slice(i,i+6).map(async file=>{
   const relative=path.relative(staticRoot,file).split(path.sep).join('/')
   const response=await fetch(base+'/'+relative.split('/').map(encodeURIComponent).join('/'))
   if(!response.ok)throw new Error('HTTP asset failed: '+relative+' '+response.status)
   const actual=hash(new Uint8Array(await response.arrayBuffer()))
   if(actual!==hash(await readFile(file)))throw new Error('HTTP asset differs: '+relative)
   const source=path.join(root,'web/dist',relative)
   if(actual!==hash(await readFile(source)))throw new Error('Build asset differs: '+relative)
   verified++
  }))
  const packs=(await(await fetch(base+'/api/story-packs')).json()).packs
  const ids={'fanren-jie':'clan-young-master','lanchuan-qunxia':'lanchuan-heir','xinghai-689':'xinghai-heir','sui-guan-realms':'sg-start-heir'}
  for(const [id,origin]of Object.entries(ids)){
   const pack=packs.find(p=>p.id===id)
   if(!pack?.creation?.origins?.some(o=>o.id===origin))throw new Error('Installed origin missing: '+origin)
   if(id==='sui-guan-realms'&&pack.creation.origins.length!==4)throw new Error('Wrong fantasy origin count')
  }
  results.push({...entry,http_assets_verified:verified,origins_verified:4})
 }finally{
  child.kill()
  await new Promise(resolve=>{if(child.exitCode!==null)resolve();else child.once('exit',resolve)})
  await writeFile(path.join(data,'startup.log'),processLog.join(''))
 }
}
await writeFile(path.join(out,'installed-http-check.json'),JSON.stringify(results,null,2))
console.log(JSON.stringify(results))
