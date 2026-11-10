import json,urllib.request,uuid
from pathlib import Path
repo=Path(__file__).resolve().parents[2]
root=repo/Path('.ta-dev/history-complete/evidence/reference-assets-movement'); result=json.loads((root/'result.json').read_text(encoding='utf-8')); route='/api/projects/'+result['project']+'/history/campaigns/'+result['campaign'];base='http://127.0.0.1:18071'
with urllib.request.urlopen(base+'/api/settings') as response: settings=json.load(response)
assert Path(settings['paths']['denova_dir']).resolve()==repo/'.ta-dev/history-complete/data'

def call(label,path,body=None):
 p=root/(label+'.json');q=root/(label+'.request.json')
 if p.exists():return json.loads(p.read_text(encoding='utf-8'))
 if q.exists():body=json.loads(q.read_text(encoding='utf-8'))['body']
 q.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False),encoding='utf-8');print('START',label,flush=True)
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','Accept-Language':'zh-CN'})
 with urllib.request.urlopen(req) as r:v=json.load(r)
 p.write_text(json.dumps(v,ensure_ascii=False,indent=2),encoding='utf-8');return v
s=call('08-before-display',route)
r=call('09-display-decree',route+'/direct-edict',{'command_id':'display-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','text':'令袁崇焕与毕自严根据已完成的山海关增援队交接，核实并更新该队现任统领、驻地和当前补给状况。只核对已存在的500人编制，不再调兵、不募兵、不新增专项支出，不动内帑。'});s=r['state']
r=call('10-display-quarter',route+'/advance',{'command_id':'display-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN'});s=r['state'];p=s['reports'][-1]['world'];changed=0
for entity in p['changes']+p['new_entities']:
 for value in (entity.get('attributes') or {}).values():
  assert value['zh'].strip() and value['en'].strip();changed+=1
assert changed>0
cold=call('11-display-cold',route);assert cold==s
(root/'display-result.json').write_text(json.dumps({'turn':s['turn'],'bilingual_attributes_checked':changed,'cold_read':True}),encoding='utf-8');print('PASS bilingual dynamic attributes',changed,flush=True)
