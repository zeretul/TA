"""HY4FREE long-campaign acceptance; only the guarded isolated instance."""
import json,tomllib,urllib.request,urllib.error,uuid
from pathlib import Path
root=Path(__file__).resolve().parents[2];batch=root/'.ta-dev/history-limits';out=batch/'evidence/model';out.mkdir(exist_ok=True)
base='http://127.0.0.1:18071'
c=tomllib.loads((batch/'data/config.toml').read_text('utf-8-sig'))
assert c['model_profiles'][0]['model']=='hy4free' and c['model_endpoints'][0]['base_url']=='http://127.0.0.1:8317/v1'
with urllib.request.urlopen(base+'/api/settings') as r:settings=json.load(r)
assert Path(settings['paths']['denova_dir']).resolve()==batch/'data'
def call(label,path,body=None,fresh=False):
 file=out/(label+'.json');intent=out/(label+'.request.json')
 if file.exists() and not fresh:return json.loads(file.read_text('utf-8'))
 if intent.exists() and not fresh:body=json.loads(intent.read_text('utf-8'))['body']
 intent.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False,indent=2),'utf-8');print('START',label,flush=True)
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','Accept-Language':'zh-CN'})
 try:
  with urllib.request.urlopen(req) as r:result=json.load(r)
 except urllib.error.HTTPError as error:
  (out/(label+'-error.json')).write_bytes(error.read());raise
 file.write_text(json.dumps(result,ensure_ascii=False,indent=2),'utf-8');print('OK',label,flush=True);return result
b=json.loads((batch/'evidence/bootstrap.json').read_text('utf-8'));f=json.loads((batch/'evidence/long-fixture.json').read_text('utf-8'));route='/api/projects/'+b['project_id']+'/history/campaigns/'+f['id']
s=call('before',route)
r=call('archive-chat',route+'/chat',{'command_id':'long-chat-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','minister_ids':['li_gang'],'message':'李纲，请查阅已离任的旧任文书699号公开履历，告诉朕该档保存的四字原文。必须从原记录查证，不猜测，不进行政务。'});s=r['state'];text=' '.join(x['text'] for x in s['chats'][-1]['replies']);assert '古井晴川' in text,text
r=call('long-quarter',route+'/advance',{'command_id':'long-quarter-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN'});s=r['state']
assert s['turn']==f['turn']+1 and len(s['world']['entities'])>=len(json.loads((out/'before.json').read_text('utf-8'))['world']['entities'])>768 and len(s['world']['accounts'])>=268
results=s['reports'][-1]['world']['results'];assert len(results)==150 and len({r['order_id'] for r in results})==150
assert all(r['status']=='waiting' and r['progress']==0 for r in results)
assert all(not any(o.get('spent',{}).values()) for o in s['world']['orders'])
assert call('cold-read',route,fresh=True)==s
(out/'result.json').write_text(json.dumps({'model':'hy4free','entities':len(s['world']['entities']),'accounts':len(s['world']['accounts']),'order_results':len(results),'old_record_found':True,'atomic_turn':s['turn'],'no_premature_execution':True},indent=2),'utf-8');print('PASS long campaign',flush=True)
