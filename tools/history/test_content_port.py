"""Exercise reference-content gameplay with the user's local model, isolated saves only."""
import argparse
import json
import time
import tomllib
import urllib.request
import urllib.error
import uuid
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument('--scenario',choices=['ming-1627','song-1127'],required=True)
parser.add_argument('--resume',action='store_true')
parser.add_argument('--run',choices=['content','final'],default='content')
args=parser.parse_args()
root=Path(__file__).resolve().parents[2]
out=root/'.ta-dev/history-complete/evidence'/(args.run+'-'+args.scenario)
out.mkdir(parents=True,exist_ok=True)
cfg=tomllib.loads((root/'.ta-dev/history-complete/data/config.toml').read_text(encoding='utf-8-sig'))
default=next(p for p in cfg['model_profiles'] if p['id']=='default')
endpoint=next(e for e in cfg['model_endpoints'] if e['id']==default['endpoint_id'])
assert endpoint['base_url'].rstrip('/')=='http://127.0.0.1:8317/v1','Only the original WorkBuddy endpoint is authorized'
assert default['model']=='hy4free','This acceptance run requires HY4FREE'
base='http://127.0.0.1:18071'
with urllib.request.urlopen(base+'/api/settings') as response:settings=json.load(response)
assert Path(settings['paths']['denova_dir']).resolve()==root/'.ta-dev/history-complete/data'

def request(label,path,body=None):
    cache=out/(label+'.json');intent=out/(label+'.request.json')
    if args.resume and cache.exists():return json.loads(cache.read_text(encoding='utf-8'))
    if args.resume and intent.exists():body=json.loads(intent.read_text(encoding='utf-8'))['body']
    intent.write_text(json.dumps({'path':path,'body':body},ensure_ascii=False,indent=2),encoding='utf-8')
    print('START',label,flush=True);start=time.monotonic()
    req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body,ensure_ascii=False).encode(),headers={'Content-Type':'application/json','Accept-Language':(body or {}).get('locale','zh-CN')})
    try:
        with urllib.request.urlopen(req) as response:result=json.load(response)
    except urllib.error.HTTPError as exc:
        (out/(label+'-error.json')).write_bytes(exc.read());raise
    cache.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8');print('OK',label,round(time.monotonic()-start,1),flush=True)
    return result

bootstrap=request('01-bootstrap','/api/history/bootstrap',{})
prefix='/api/projects/'+bootstrap['project_id']+'/history/campaigns'
state=request('02-create',prefix,{'name':'内容移植隔离 · '+args.scenario,'scenario_id':args.scenario,'locale':'zh-CN','command_id':'port-'+uuid.uuid4().hex})
route=prefix+'/'+state['id']
def act(label,action,fields):
    global state
    result=request(label,route+'/'+action,{'command_id':'port-'+uuid.uuid4().hex,'expected_revision':state['revision'],'locale':'zh-CN',**fields})
    if 'state' in result:state=result['state']
    return result

ming=args.scenario=='ming-1627'
investigator='port-qiao-yunsheng' if ming else 'li_gang'
maker='port-dong-kewei' if ming else 'zong_ze'
act('03-private','chat',{'minister_ids':[investigator],'message':'只在私下记住核查代号“绛雪照仓”。先说说如何区分钱粮账目中的传闻、可核查线索和定罪证据，不得说已经调查或提前抄家。'})
if ming:
    decree='密诏命乔允升以“绛雪照仓”为代号，秘密查核陕西一处赈济粮册与实际放粮，最多动国库3000两，不准无据定罪和抄家，保存来源与查证结果，密奏。另命董可威于遵化铁厂利用现有工匠和设备，以现存矿石2000千克试炼铁料1000千克，开支不超过5000两，核实产出，不修新厂；命徐光启核查农业试种所需人才、材料和已有技术，做小规模可负担试验，限4000两，不得把试验当全国推广。命毕自严将陕西赈济持续办理，累计银不超过15000两、粮不超过2000石，不动内帑。命袁崇焕核查宁远军现有兵员、欠饷与补给并原地整训，限银6000两、粮1000石，不扩军不越境。'
else:
    decree='密诏命李纲以“绛雪照仓”为代号，秘密核查两淮一处军粮发放的账实，限钱3000贯，保留证据密奏，不准无据抄家。命宗泽督办江东军器作院以现存原料试制一批弓弩甲械，限钱5000贯，记录原料消耗和真实产出，不套用明代火铳技术。命李纲持续整顿两淮江防，累计钱不超过20000贯、粮不超过4000石。另命许景衡梳理当前税赋实际征解与欠款，限钱1000贯，不能将未收到的税款算进钱库。'
initial={a['id']:a['balance'] for a in state['world']['accounts']}
act('04-direct','direct-edict',{'text':decree})
assert {a['id']:a['balance'] for a in state['world']['accounts']}==initial,'Issuing a decree spent resources early'
secret=next(o for o in state['world']['orders'] if '绛雪照仓' in o['text'])
assert secret.get('disclosure')=='sealed','Secrecy was not understood or persisted'
act('05-quarter','advance',{})
assert state['turn']==1 and all(a['balance']>=0 for a in state['world']['accounts'])
if ming:assert next(a['balance'] for a in state['world']['accounts'] if a['id']=='privy')==initial['privy']
assert state['reports'][-1]['narrative']
for event in state['world']['events']:
    if secret['id'] in event.get('order_ids',[]):assert 'public' not in event['audience']
results={r['order_id']:r for r in state['reports'][-1]['world']['results']}
assert secret['id'] in results and results[secret['id']]['reason']
act('06-council','chat',{'minister_ids':[investigator,maker],'message':'本次公开廷议仅讨论已经公开的生产、财政与军需情况。请分别说明本职可确认的事实、执行瓶颈与下一步建议，不谈私下事项。'})
assert '绛雪照仓' not in '\n'.join(r['text'] for r in state['chats'][-1]['replies']),'Secret leaked into public council'
act('07-followup','chat',{'minister_ids':[investigator],'source_report_id':state['reports'][-1]['id'],'message':'现在私下核对本季密查：先答我们的代号，再说查实什么、仍有什么未知、何种证据可以作数和实际花费；不要把推测当成定罪结论。'})
assert '绛雪照仓' in state['chats'][-1]['replies'][0]['text'],'Private memory lost'
if ming:
    next_decree='命毕自严核对剩余额度，原有赈济在额度内继续，无须重发。命乔允升会同工部尚书董可威对已发现的真实账实问题提出处置，先保全证据；未查实的不定罪。命孙承宗择合适使者就朝鲜现有关系议一项互通物资的有限协定，不割地，不许大额岁币，限银3000两；对方未接受前不称签约。命礼部核实皇室已出生人选、健康与继承礼制，拟定储位和教育建议，未获批不擅立储，不凭空生造成年太子。'
else:
    next_decree='原有江防按剩余额度继续。命李纲根据已经查实的军粮情况处理差额并留下来源，不把传闻当定罪依据。命黄潜善择使者就当前外部势力议一项有限的停战或贸易条件，限钱3000贯，不割地、不承诺巨额岁币，对方未接受不称签约。命许景衡核实皇室已出生人选、健康与继承礼制，拟定储位和教育建议，未获批不擅立储。'
act('08-next-decree','direct-edict',{'text':next_decree})
act('09-next-quarter','advance',{})
assert state['turn']==2
saved=json.loads((out/'09-next-quarter.request.json').read_text(encoding='utf-8'))
duplicate=request('10-retry',saved['path'],saved['body'])
assert duplicate['duplicate'] and duplicate['state']['turn']==2
act('11-english','chat',{'minister_ids':[investigator],'locale':'en-US','message':'Answer in English. Summarize the current treasury balance, the status of your assigned work, and what remains uncertain. Distinguish completed results from proposals.'})
import re
if re.search('[\u4e00-\u9fff]',state['chats'][-1]['replies'][0]['text'].replace('绛雪照仓','')):
    act('11-english-retest','chat',{'minister_ids':[investigator],'locale':'en-US','message':'Rewrite your previous answer entirely in English, translating every ordinary administrative term into English. Preserve the recorded facts and uncertainty. Do not add new claims. Only the exact private code name may be quoted in its original language.'})
assert not re.search('[\u4e00-\u9fff]',state['chats'][-1]['replies'][0]['text'].replace('绛雪照仓','')),'English reply contains untranslated Chinese'
cold=request('12-cold',route)
assert cold==state
summary={'scenario':args.scenario,'project':bootstrap['project_id'],'campaign':state['id'],'turns':state['turn'],'sealed_order':secret['id'],'portrait_count':len(state['scenario']['presentation']['portraits']),'ministers':len(state['ministers']),'secret_council_leak':False,'duplicate_turn':False,'endpoint':'WorkBuddy 8317','model':default['model'],'scenario_digest':state['scenario_digest'],'accounts':state['world']['accounts']}
(out/'result.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8')
print('PASS',args.scenario,'content port',flush=True)
