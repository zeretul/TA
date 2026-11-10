"""Review-closure acceptance through the user's WorkBuddy HY4FREE only."""
import json,uuid,tomllib,urllib.request,urllib.error
from pathlib import Path
root=Path(__file__).resolve().parents[2];out=root/'.ta-dev/history-complete/evidence/closure-model';out.mkdir(parents=True,exist_ok=True)
base='http://127.0.0.1:18071'
c=tomllib.loads((root/'.ta-dev/history-complete/data/config.toml').read_text(encoding='utf-8-sig'));profile=next(p for p in c['model_profiles'] if p['id']=='default');endpoint=next(e for e in c['model_endpoints'] if e['id']==profile['endpoint_id'])
assert profile['model']=='hy4free' and endpoint['base_url']=='http://127.0.0.1:8317/v1'
with urllib.request.urlopen(base+'/api/settings') as r:settings=json.load(r)
assert Path(settings['paths']['denova_dir']).resolve()==root/'.ta-dev/history-complete/data'
def call(label,path,body=None,refresh=False):
 file=out/(label+'.json');intent=out/(label+'.request.json')
 if file.exists() and not refresh:return json.loads(file.read_text(encoding='utf-8'))
 if intent.exists():body=json.loads(intent.read_text(encoding='utf-8'))['body']
 intent.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False,indent=2),encoding='utf-8');print('START',label,flush=True)
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','Accept-Language':'zh-CN'})
 try:
  with urllib.request.urlopen(req) as r:value=json.load(r)
 except urllib.error.HTTPError as error:
  (out/(label+'-error.json')).write_bytes(error.read());raise
 file.write_text(json.dumps(value,ensure_ascii=False,indent=2),encoding='utf-8');print('OK',label,flush=True);return value
b=call('bootstrap','/api/history/bootstrap',{},refresh=True);prefix='/api/projects/'+b['project_id']+'/history/campaigns'
results=[]
for name,scenario,cap in [('low','ming-1627',100),('high','ming-1627',1000),('song','song-1127',1000)]:
 s=call(name+'-create',prefix,{'command_id':'closure-'+uuid.uuid4().hex,'name':'审查闭环隔离 '+name,'scenario_id':scenario,'locale':'zh-CN'});route=prefix+'/'+s['id'];unit='两' if scenario=='ming-1627' else '贯';minister='毕自严' if scenario=='ming-1627' else '李纲'
 text=f'命{minister}安排两名现有书吏，将一册已经齐备的公开仓储账簿誊清，只抄写，不追加调查、外派或采购。每名书吏另付工钱每日1{unit}，工作6日，本季完成；纸墨使用已有库存。该项临时工钱按实际人数和工日支付，累计上限{cap}{unit}，上限不是应花金额，余款不花，不增加范围，不动皇室私款。'
 if name=='low':text+='另由国库赏魏忠贤个人银80两，只付这一次，累计上限100两，记为对个人的真实付款，不能按内部拨款计为零。'
 r=call(name+'-edict',route+'/direct-edict',{'command_id':'closure-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','text':text});s=r['state']
 r=call(name+'-quarter',route+'/advance',{'command_id':'closure-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN'});s=r['state'];resource='silver' if scenario=='ming-1627' else 'cash'
 copy=next(o for o in s['world']['orders'] if '抄' in o['text'] or '誊' in o['text']);spent=copy['spent'].get(resource,0);assert spent==12,(name,'copy cost',spent)
 if name=='low':
  reward=next(o for o in s['world']['orders'] if '魏忠贤' in o['text']);assert reward['spent'].get(resource)==80
 assert call(name+'-cold',route)==s
 results.append({'name':name,'campaign':s['id'],'scenario':scenario,'cap':cap,'cost':spent,'project':b['project_id']})
 if name=='low':
  r=call('advice-chat',route+'/chat',{'command_id':'advice-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','minister_ids':['bi-ziyan'],'message':'请建议下一季如何继续核对公开账目，不增支出。现在只给一个短建议，不执行。'});s=r['state'];chat=s['chats'][-1]
  r=call('advice-save',route+'/advice',{'command_id':'advice-'+uuid.uuid4().hex,'expected_revision':s['revision'],'advice':{'action':'save','chat_id':chat['id'],'minister_id':'bi-ziyan','title':'下季公开账目核对','notes':'待审议，未颁行'}});s=r['state'];a=s['advice'][-1]
  prepared=call('advice-prepare',route+'/prepare-edict',{'command_id':'advice-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','text':'依据所选收藏建议拟诏，不扩范围，不追加预算。','source_advice_id':a['id']})
  assert prepared['plan']['source_advice_id']==a['id'] and prepared['plan']['source_chat_id']==chat['id']
  assert call('advice-before-issue',route)==s
  r=call('advice-issue',route+'/edict',{'command_id':'advice-'+uuid.uuid4().hex,'expected_revision':s['revision'],'plan':prepared['plan']});s=r['state'];assert s['edicts'][-1]['plan']['source_advice_id']==a['id']
  call('advice-final-cold',route)
# Follow the selected suggestion through its scheduled execution, not only issue.
issued=json.loads((out/'advice-issue.json').read_text(encoding='utf-8'))['state']
route=prefix+'/'+issued['id'];advice_order=issued['world']['orders'][-1]['id']
for label in ['advice-scheduled-wait','advice-execution']:
 current=next(o for o in issued['world']['orders'] if o['id']==advice_order)
 if label=='advice-scheduled-wait' and issued['turn']>=current.get('not_before_turn',0):continue
 r=call(label,route+'/advance',{'command_id':'advice-run-'+uuid.uuid4().hex,'expected_revision':issued['revision'],'locale':'zh-CN'});issued=r['state']
 current=next(o for o in issued['world']['orders'] if o['id']==advice_order)
 assert not any(current['spent'].values()),'Advice spent new funds without authorization'
 for old in issued['world']['orders'][:2]:
  assert old['status']=='completed' and old['spent'].get('silver') in [12,80],'Completed decree repeated expenditure'
 if label=='advice-execution':assert current['status']=='completed','Scheduled clerical advice did not complete'
assert call('advice-execution-cold',route)==issued
advice_execution={'campaign':issued['id'],'turn':issued['turn'],'order':advice_order,'status':current['status'],'new_spending':current['spent'],'old_one_shots_unchanged':True}
(out/'advice-execution-result.json').write_text(json.dumps(advice_execution,ensure_ascii=False,indent=2),encoding='utf-8')
(out/'result.json').write_text(json.dumps({'model':'hy4free','endpoint':'WorkBuddy 8317','cases':results,'saved_advice_source_verified':True},ensure_ascii=False,indent=2),encoding='utf-8');print('PASS review closure model tests',flush=True)
