"""Produce independent, reviewable English candidates using the existing local client."""
import argparse, concurrent.futures, importlib.util, json, re, time
from pathlib import Path

p=argparse.ArgumentParser();p.add_argument('--source-root',type=Path,required=True);p.add_argument('--evidence',type=Path,required=True);p.add_argument('--only',default='');p.add_argument('--force',action='store_true');p.add_argument('--source-json',default='source-records.json');p.add_argument('--output-prefix',default='english');a=p.parse_args()
spec=importlib.util.spec_from_file_location('local_cpa_client',a.source_root/'work/ta-formal-full-review-20261026/cpa-client.py')
client=importlib.util.module_from_spec(spec);spec.loader.exec_module(client)
records=json.loads((a.evidence/a.source_json).read_text(encoding='utf-8'))
items=[dict(id=r['id'],name=r['name'],content=r['content']) for rs in records.values() for r in rs]
groups=[];batch=[];size=0
for item in items:
    if size+len(item['content'])>6500 and batch:groups.append(batch);batch=[];size=0
    batch.append(item);size+=len(item['content'])
if batch:groups.append(batch)
system='''你是文字冒险世界的专业英文本地化编辑。把用户提供的每个条目的name和content完整译为自然英文，不删段落、秘密、限制、条件、事实和数字，不添加游戏效果。Markdown结构可保留。稳定id、引用编号(P01/I02等)、已有英文名和数字保持不变。人名地名统一：澜川=Lanchuan，潮信渡=Tidewatch Ferry，宋老橹=Song Laolu，蓝阿鲤=Lan Ali，朱伯利=Juberly，韩萧=Han Xiao，黑星=Black Star，苏尼尔=Sunil，歌朵拉=Godora，龙坦=Floating Dragon，瑞斯达=Risda，弗丁=Feidin，晨星=Morning Star。只返回JSON {"items":[{"id":"原id","name":"English name","content":"Complete English translation"}]}，全部ID必须与输入一一对应。不要输出解释。'''

def run(index,group):
    target=a.evidence/f'{a.output_prefix}-candidate-{index:03}.json'
    if target.exists() and not a.force:
        old=json.loads(target.read_text(encoding='utf-8'))
        if old.get('ok') and {x['id'] for x in old['items']}=={x['id'] for x in group}:return dict(batch=index,cached=True,items=len(group))
    began=time.monotonic()
    try:
        result=client.generate(system,json.dumps({'items':group},ensure_ascii=False),max_tokens=24000)
        (a.evidence/f'{a.output_prefix}-response-{index:03}.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
        text=re.sub(r'^```(?:json)?\s*|\s*```$','',result['text'].strip())
        data=json.loads(text)
        assert {x['id'] for x in data['items']}=={x['id'] for x in group}
        assert len(data['items'])==len(group)
        for item in data['items']:assert item['name'].strip() and item['content'].strip()
        record=dict(ok=True,items=data['items'],model=result['reported_model'],usage=result['usage'],finish_reason=result['finish_reason'],seconds=round(time.monotonic()-began,2))
    except Exception as e:
        record=dict(ok=False,error=type(e).__name__,http_status=getattr(e,'code',None),seconds=round(time.monotonic()-began,2))
    target.write_text(json.dumps(record,ensure_ascii=False,indent=2),encoding='utf-8')
    return dict(batch=index,ok=record['ok'],items=len(group),seconds=record['seconds'])

print(json.dumps(dict(batches=len(groups),items=len(items))),flush=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    selected={int(x) for x in a.only.split(',') if x}
    futures=[pool.submit(run,i,g) for i,g in enumerate(groups,1) if not selected or i in selected]
    for f in concurrent.futures.as_completed(futures):print(json.dumps(f.result()),flush=True)
