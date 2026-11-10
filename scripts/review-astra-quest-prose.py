"""Create independent branch-specific prose candidates; never write quest assets."""
import concurrent.futures, importlib.util, json, re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[4]
OUT=ROOT/'work/astra-completion-20261102/content'
spec=importlib.util.spec_from_file_location('local_client',ROOT/'work/ta-formal-full-review-20261026/cpa-client.py')
client=importlib.util.module_from_spec(spec);spec.loader.exec_module(client)
records=json.loads((OUT/'source-records.json').read_text(encoding='utf8'))
groups=[]
for prefix in records:
    source={r['id']:r for r in records[prefix]}
    catalog=json.loads((OUT/f'{prefix}-quest-catalog.json').read_text(encoding='utf8'))
    jobs=[dict(source=source[q['id']],definition=q) for q in catalog if re.fullmatch(prefix+r'-a\d{2}',q['id'])]
    groups.extend([jobs[i:i+4] for i in range(0,len(jobs),4)])
SYSTEM='''你是中文互动叙事编辑。任务是审核正式任务分支文案。输入原稿source与当前正式规则definition。为每个choice返回独立中文自然文字，不更改任何规则。仅输出JSON {"items":[{"id":"choice稳定ID","title":"10至22字完整动作短标题","description":"玩家本次具体行动","success":"该分支成功后眼前真实结果","failure":"该分支失败后的具体情景","success_echo":"该分支成功的后续回响","failure_echo":"该分支失败的后续回响","review_note":"任何规则与原稿冲突或不能文案修复的问题"}]}。每choice必有一项，保留稳定ID。绝不把原稿三种互斥方案的结果同时授予同一选项；例如救药不能同时被全星系追杀，仅陪烟不等于父女和解。去掉独立耗材/不同于I32/已完成/此前你选择/规则等工程语言。禁止省略截断标题。正式消耗、奖励、耗时及已登记机制优先；没有规则消费者不得承诺免费交通、全体好感、即时重伤/死亡、奖励凭空到账或百分比buff。只可写NPC当下可观察反应、消息、态度和剧情挂钩。如果正式奖励本身不合理，不修改数字、不补剧情强圆，在review_note指出。success不得把只尝试调查说成全案侦破。failure不能给成功奖励，不新增规则未执行的损失。echo避免保证某关系/身份变化，给可继续行动的具体线索。保留正常人性、明确分支差别、原作名字与线索。'''
def run(i,group):
    target=OUT/f'quest-prose-candidate-{i:02}.json'
    if target.exists():
        old=json.loads(target.read_text(encoding='utf8'))
        if old.get('ok'):return old
    response=client.generate(SYSTEM,json.dumps(group,ensure_ascii=False),max_tokens=18000)
    (OUT/f'quest-prose-response-{i:02}.json').write_text(json.dumps(response,ensure_ascii=False,indent=2),encoding='utf8')
    data=json.loads(re.sub(r'^```(?:json)?\s*|\s*```$','',response['text'].strip()))
    expected={c['id'] for q in group for c in q['definition']['choices']}
    assert {c['id'] for c in data['items']}==expected
    data.update(ok=True,model=response['reported_model'])
    target.write_text(json.dumps(data,ensure_ascii=False,indent=2),encoding='utf8')
    print(json.dumps(dict(batch=i,items=len(data['items']),ok=True)),flush=True)
    return data
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    results=list(pool.map(lambda x:run(*x),enumerate(groups,1)))
merged={r['id']:{k:v for k,v in r.items() if k!='id'} for result in results for r in result['items']}
(OUT/'quest-prose-reviewed.json').write_text(json.dumps(merged,ensure_ascii=False,indent=2),encoding='utf8')
print(json.dumps(dict(total=len(merged))),flush=True)
