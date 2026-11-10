"""Final-reference smoke test: private asset visibility and real troop movement."""
import argparse
import json
import urllib.request
import uuid
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument('--resume',action='store_true')
args=parser.parse_args()
root=Path(__file__).resolve().parents[2]
out=root/'.ta-dev/history-complete/evidence/reference-assets-movement'
out.mkdir(parents=True,exist_ok=True)
base='http://127.0.0.1:18071'
with urllib.request.urlopen(base+'/api/settings') as response:settings=json.load(response)
assert Path(settings['paths']['denova_dir']).resolve()==root/'.ta-dev/history-complete/data'

def request(label,path,body=None):
    saved=out/(label+'.json');intent=out/(label+'.request.json')
    if args.resume and saved.exists():return json.loads(saved.read_text(encoding='utf-8'))
    if args.resume and intent.exists():body=json.loads(intent.read_text(encoding='utf-8'))['body']
    intent.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False,indent=2),encoding='utf-8')
    print('START',label,flush=True)
    req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body,ensure_ascii=False).encode(),headers={'Content-Type':'application/json','Accept-Language':'zh-CN'})
    with urllib.request.urlopen(req) as response:result=json.load(response)
    saved.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8');print('OK',label,flush=True);return result

b=request('01-bootstrap','/api/history/bootstrap',{})
prefix='/api/projects/'+b['project_id']+'/history/campaigns'
s=request('02-create',prefix,{'name':'最终资料与军队调动隔离验收','scenario_id':'ming-1627','command_id':'assets-'+uuid.uuid4().hex,'locale':'zh-CN'})
assert len(s['ministers'])==102 and len(s['scenario']['presentation']['portraits'])==119
assert not any(a['id'].startswith('port-estate-') for a in s['world']['accounts']),'Private estates were revealed at opening'
assert any(e['id']=='port-tian-ergeng' for e in s['world']['entities'])
route=prefix+'/'+s['id']
opening=next(e['metrics']['size'] for e in s['world']['entities'] if e['id']=='army-1')
privy=next(a['balance'] for a in s['world']['accounts'] if a['id']=='privy')
decree='命袁崇焕从宁远军现有兵员中抽调500人，沿现有道路赴北直隶山海关，与当地防务交接。原宁远军扣除调出兵员并留守，这500人单独编为一支山海关增援队，仍归袁崇焕统辖，另记其编制与驻地，不新募兵、不重复计数，限国库银5000两、粮500石。按实际路程和军需行军，若未到则记在途，不假称瞬移。另命毕自严核实国库与太仓现存账目，整理公开财政奏报，不动内帑。对魏忠贤、崔呈秀等人的资产只核查权属与来源，不按有财产认定犯罪，不予扣押，不向国库转入私人资产。'
r=request('03-decree',route+'/direct-edict',{'command_id':'assets-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','text':decree});s=r['state']
r=request('04-quarter',route+'/advance',{'command_id':'assets-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN'});s=r['state']
assert s['turn']==1
assert next(a['balance'] for a in s['world']['accounts'] if a['id']=='privy')==privy
army=next(e for e in s['world']['entities'] if e['id']=='army-1')
new_forces=[e for e in s['reports'][-1]['world']['new_entities'] if e['kind']=='army']
assert new_forces,'No troop detachment was persisted'
assert any(e['metrics'].get('size')==500 for e in new_forces),'Detachment size was not persisted'
assert army['metrics']['size']<=opening-500,'Parent army did not release the detachment'
assert not any(t['to']=='treasury' and t['from'].startswith('port-estate-') for t in s['reports'][-1]['world']['transfers']),'Unapproved confiscation'
detachment_id=next(e['id'] for e in new_forces if e['metrics'].get('size')==500)
followup='继续办理已保存的山海关增援队500人交接，只推进这一支在途部队，不再从宁远军抽兵，不新募兵。由袁崇焕督办，以北直隶山海关为驻地完成交接；另追加国库银3000两、太仓粮300石用于续程及交接，按实际所需支出，余款不强行花完，不动内帑。已耗尽的前期预算不得重复支取，交接完结后在部队资料记下驻地并结束行军。资产仍只核查，不得扣押或没收。'
r=request('04b-continue',route+'/direct-edict',{'command_id':'arrival-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN','text':followup});s=r['state']
r=request('04c-arrival-quarter',route+'/advance',{'command_id':'arrival-'+uuid.uuid4().hex,'expected_revision':s['revision'],'locale':'zh-CN'});s=r['state']
detachment=next(e for e in s['world']['entities'] if e['id']==detachment_id)
entities={e['id']:e for e in s['world']['entities']}
location=detachment.get('location');seen=set()
while location and location!='beizhili' and location not in seen:
    seen.add(location);location=entities.get(location,{}).get('location')
assert location=='beizhili','Detachment has not reached its recorded destination'
assert detachment['metrics']['size']==500
army=entities['army-1']
assert army['metrics']['size']==opening-500,'Continuation detached the same soldiers again'
assert next(a['balance'] for a in s['world']['accounts'] if a['id']=='privy')==privy
assert not any(t['to']=='treasury' and t['from'].startswith('port-estate-') for t in s['reports'][-1]['world']['transfers'])
cold=request('05-cold',route)
assert cold==s
(out/'result.json').write_text(json.dumps({'project':b['project_id'],'campaign':s['id'],'scenario_digest':s['scenario_digest'],'turn':s['turn'],'ministers':len(s['ministers']),'portraits':len(s['scenario']['presentation']['portraits']),'detachment':detachment,'parent_army_size':army['metrics']['size'],'private_assets_hidden_at_opening':True,'privy_preserved':True},ensure_ascii=False,indent=2),encoding='utf-8')
song=request('06-song-create',prefix,{'name':'南宋宰执钱粮知情隔离验收','scenario_id':'song-1127','command_id':'song-brief-'+uuid.uuid4().hex,'locale':'zh-CN'})
song_route=prefix+'/'+song['id']
reply=request('07-song-finance',song_route+'/chat',{'command_id':'song-brief-'+uuid.uuid4().hex,'expected_revision':song['revision'],'locale':'zh-CN','minister_ids':['li_gang'],'message':'请按当前已核定账目，用阿拉伯数字准确报告行朝钱库和粮储余额及单位，区分余额与未来压力，不使用旧奏报代替当前余额。'})
speech=reply['state']['chats'][-1]['replies'][0]['text'].replace(',','').replace('，','')
assert '1500000' in speech and '1200000' in speech,'Song chancellor did not read current public accounts'
(out/'song-finance-result.json').write_text(json.dumps({'project':b['project_id'],'campaign':song['id'],'scenario_digest':song['scenario_digest'],'treasury':1500000,'grain':1200000,'reply_verified':True},ensure_ascii=False,indent=2),encoding='utf-8')
print('PASS reference assets, troop movement and Song current accounts',flush=True)
