"""Reviewable Chinese/English adaptations for every original skill and item."""
import argparse, concurrent.futures, importlib.util, json, re, time, hashlib
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--source-root',type=Path,required=True);p.add_argument('--evidence',type=Path,required=True);a=p.parse_args()
spec=importlib.util.spec_from_file_location('client',a.source_root/'work/ta-formal-full-review-20261026/cpa-client.py');client=importlib.util.module_from_spec(spec);spec.loader.exec_module(client)
records=json.loads((a.evidence/'source-records.json').read_text(encoding='utf-8'))
items=[]
for prefix,rows in records.items():
    rules=json.loads((a.evidence/f'{prefix}-adventure-rules.json').read_text(encoding='utf-8'))
    mechanics={r['id']:r for r in rules['items']+rules['skills']}
    for r in rows:
        if r['source_id'][0] in 'SI':items.append(dict(id=r['id'],name=r['name'],source=r['content'],mechanics=mechanics[r['id']]))
groups=[items[i:i+12] for i in range(0,len(items),12)]
system='''你是文字冒险《Text Adventure》的中英文内容编辑，负责把原稿能力/物品打磨成已经接通规则的可玩改编。逐项保留名字、来历、获取出处、风格、材料外观、重要人际线索，但效果仅写mechanics真实允许的行为。不把工程限制、ID、字段名、代码、适配说明写进玩家正文。source原文包含未实现或夸大的承诺，须审慎改写：不能写未支持的免疫、反射、必中、即死、永久属性、固定百分比、行动以外的隐藏触发、跨战斗规模伤害。不要将工程/学识说成伤害法术。
真实规则：attack攻击；heal恢复生命（不是消除所有重伤）；energy恢复体力/内力；antidote清除当前战斗中毒（不是永久免疫）；guard加强本轮防御；control使敌人本轮不能反击；poison令同一场战斗持续掉生命；repair修复机械/舰船耐久；engineering仅相关制作或委托资格；pilot战中防御与相关委托资格；social只相关任务的知识/交涉资格，不保证交涉结果。effects非空就是本技能同时实际效果。cost为一次消耗，power不要写进散文；scale person/machine/ship分开，绝不把人拳伤害扩大为毁灭舰队。学习须有实际传授/手札并完成研习；原稿几天/多年若与mechanics小时不同，不承诺原数值。
物品：weapon/armor装备后分别助攻击/防御；tool无effect只用于实际任务的线索/资格，不能承诺额外数值；material是制作或委托所需材料（未提供具体配方时不编配方）；consumable只effect所述作用一次；machine/ship可部署、操作、维护，要有耐久和燃料，不免除驾驶/任务要求。effect revive为装备后在一次个人战斗的致命伤时按chance百分比尝试生还，成功或失败都消耗，成功脱离本场，不能抵消真空/持续高温；compute为装在舰船工具槽后使舰船技能效力乘multiplier，不把整个宇宙黑客能力翻倍。acquisition reward物品只在相应事件真正取得，不可直接购买，unique只能一次获得。不得创造新人物/奖励/定价/状态/能力。
每条用自然中文约150-350字（按内容需要），分2-3段，明确可用效果及如何得到/学习。英文全文准确对应，不夹中文原文。brief是公开一行（20-60汉字及对应英文），不提前揭露秘密。只返回JSON {"items":[{"id":"原ID","content_zh":"中文正文","content_en":"English body","name_en":"English name","brief_zh":"中文简介","brief_en":"English brief","adjustments":"审稿备注：被改变的原稿效果；不进入正文"}]}。完整一一对应输入所有ID。'''

def run(index,group):
    target=a.evidence/f'effect-candidate-{index:03}.json';digest=hashlib.sha256(json.dumps(group,ensure_ascii=False,sort_keys=True).encode()).hexdigest()
    if target.exists():
        old=json.loads(target.read_text(encoding='utf-8'))
        if old.get('ok') and old.get('input_hash')==digest:return dict(batch=index,cached=True)
    began=time.monotonic();raw=None
    try:
        raw=client.generate(system,json.dumps({'items':group},ensure_ascii=False),max_tokens=20000)
        text=re.sub(r'^```(?:json)?\s*|\s*```$','',raw['text'].strip());result=json.loads(text)
        assert len(result['items'])==len(group) and {x['id'] for x in result['items']}=={x['id'] for x in group}
        for row in result['items']:
            assert all(row.get(k,'').strip() for k in ('content_zh','content_en','name_en','brief_zh','brief_en','adjustments'))
        record=dict(ok=True,input_hash=digest,items=result['items'],model=raw['reported_model'],usage=raw['usage'],seconds=round(time.monotonic()-began,2))
    except Exception as e:record=dict(ok=False,input_hash=digest,error=type(e).__name__,http_status=getattr(e,'code',None),seconds=round(time.monotonic()-began,2))
    if raw:(a.evidence/f'effect-response-{index:03}.json').write_text(json.dumps(raw,ensure_ascii=False,indent=2),encoding='utf-8')
    target.write_text(json.dumps(record,ensure_ascii=False,indent=2),encoding='utf-8')
    return dict(batch=index,ok=record['ok'],seconds=record['seconds'])
print(json.dumps(dict(items=len(items),batches=len(groups))),flush=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    for f in concurrent.futures.as_completed([pool.submit(run,i,g) for i,g in enumerate(groups,1)]):print(json.dumps(f.result()),flush=True)
