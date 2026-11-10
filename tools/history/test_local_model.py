"""Opt-in integration exercise against an already isolated local TA server.

The script does not change credentials or log settings responses. HTTP requests
use the application's configured model and create their own named campaign.
"""
import argparse
import json
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument('--url',default='http://127.0.0.1:18071')
parser.add_argument('--evidence',required=True)
parser.add_argument('--scenario',default='ming-1627')
parser.add_argument('--quarters',type=int,default=2)
parser.add_argument('--resume',action='store_true')
args=parser.parse_args()
if not args.url.startswith('http://127.0.0.1:'):raise SystemExit('Use an isolated loopback server')
evidence=Path(args.evidence);evidence.mkdir(parents=True,exist_ok=True)
counter=0
def request(path,body=None,label='request'):
    global counter
    counter+=1;started=time.monotonic();print(f'START {label}',flush=True)
    saved=evidence/f'{counter:02d}-{label}.json'
    intent=evidence/f'{counter:02d}-{label}.request.json'
    if args.resume and intent.exists():body=json.loads(intent.read_text(encoding='utf-8'))['body']
    request.last_body=body
    if args.resume and saved.exists():
        print(f'RESUME {label}',flush=True);return json.loads(saved.read_text(encoding='utf-8'))
    intent.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False,indent=2),encoding='utf-8')
    req=urllib.request.Request(args.url+path,data=json.dumps(body,ensure_ascii=False).encode() if body is not None else None,headers={'Content-Type':'application/json','Accept-Language':'zh-CN'})
    try:
        with urllib.request.urlopen(req) as response:result=json.load(response)
    except urllib.error.HTTPError as exc:
        error=exc.read().decode('utf-8');(evidence/f'{counter:02d}-{label}-error.json').write_text(error,encoding='utf-8');print(f'FAIL {label} HTTP {exc.code}: {error}',flush=True);raise
    (evidence/f'{counter:02d}-{label}.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
    print(f'OK {label} {time.monotonic()-started:.1f}s',flush=True);return result

with urllib.request.urlopen(args.url+'/api/settings') as response:settings=json.load(response)
expected_data=Path(__file__).resolve().parents[2]/'.ta-dev/history-complete/data'
if Path(settings['paths']['denova_dir']).resolve()!=expected_data.resolve():raise SystemExit('Refusing to test a non-isolated application')
bootstrap=request('/api/history/bootstrap',{},'bootstrap')
prefix='/api/projects/'+bootstrap['project_id']+'/history/campaigns'
state=request(prefix,{'name':'隔离验收 · '+args.scenario,'scenario_id':args.scenario,'locale':'zh-CN','command_id':'test-'+uuid.uuid4().hex},'create')
campaign=prefix+'/'+state['id']
def act(action,fields=None,label=None):
    global state
    body={'command_id':'test-'+uuid.uuid4().hex,'expected_revision':state['revision'],'locale':'zh-CN',**(fields or {})}
    result=request(campaign+'/'+action,body,label or action)
    if 'state' in result:state=result['state']
    return result,request.last_body

who='bi-ziyan' if args.scenario=='ming-1627' else 'li_gang'
act('chat',{'minister_ids':[who],'message':'先记住朕给你的密语“青瓷灯照民生”，以后私下提及时要记得。现在请结合国库、军需和民生，提出两项当前能办的建议，明确代价，不要宣称已经执行。'},'private-advice')
if args.scenario=='ming-1627':
    decree='令毕自严负责陕西赈济，作为持续政策执行，每季按灾情酌情拨款发粮，累计银不超过60000两、粮不超过12000石，不动内帑；请孙承宗清点辽东军队和欠饷，先完成清点再酌拟练兵方案，未经再准不得支出；同时工部评估番薯推广所需人才与试种条件，先调查，不直接宣称技术成功。'
else:
    decree='令李纲持续整顿两淮江防，先核实军需与补给，按实际情势安排小规模修缮和训练，累计钱不超过40000贯、粮不超过10000石；同时查明行朝当前财赋来源，不得套用明代官署，不得无令迁都。'
act('direct-edict',{'text':decree},'direct-decree')
assert state['world']['orders'],'No durable orders'
starting_privy=next((a['balance'] for a in state['world']['accounts'] if a['id']=='privy'),None)
for index in range(args.quarters):
    before_turn=state['turn']
    result,body=act('advance',label=f'quarter-{index+1}')
    assert state['turn']==before_turn+1,'Clock did not advance once'
    assert all(a['balance']>=0 for a in state['world']['accounts']),'Negative resource balance'
    if starting_privy is not None:assert next(a['balance'] for a in state['world']['accounts'] if a['id']=='privy')==starting_privy,'Privy funds used without permission'
    repeat=request(campaign+'/advance',body,f'quarter-{index+1}-retry')
    assert repeat['duplicate'] and repeat['state']['turn']==state['turn'],'Duplicate settlement'
    report=state['reports'][-1]
    assert report.get('world') and report['narrative'],'Missing report or accounting'
    act('chat',{'minister_ids':[who],'source_report_id':report['id'],'message':'根据这份奏报解释本季已经做了什么、什么尚未做，下一季原有政策是否继续？再告诉朕之前私下约定的密语。'},f'report-followup-{index+1}')
    if index==0:
        checkpoint,_=act('checkpoints',{'label':'第一季之后'},'checkpoint')
        policy=next(o for o in state['world']['orders'] if o['kind']=='policy')
        act('edict',{'plan':{'text':'暂缓本项政策','instructions':[],'orders':[{'action':'pause','order_id':policy['id']}]}},'pause-policy')
        assert next(o for o in state['world']['orders'] if o['id']==policy['id'])['status']=='paused'
        act('edict',{'plan':{'text':'恢复该政策，沿用原有额度','instructions':[],'orders':[{'action':'resume','order_id':policy['id']}]}},'resume-policy')
request(campaign,label='cold-read')
(evidence/'result.json').write_text(json.dumps({'scenario':args.scenario,'campaign':state['id'],'project':bootstrap['project_id'],'turn':state['turn'],'revision':state['revision'],'accounts':state['world']['accounts'],'orders':state['world']['orders']},ensure_ascii=False,indent=2),encoding='utf-8')
print('PASS local model workflow',flush=True)
