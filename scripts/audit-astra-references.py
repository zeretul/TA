"""Semantic reference cross-check; output is review evidence, never a pack patch."""
import argparse,importlib.util,json,re
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--source-root',type=Path,required=True);p.add_argument('--evidence',type=Path,required=True);a=p.parse_args()
spec=importlib.util.spec_from_file_location('client',a.source_root/'work/ta-formal-full-review-20261026/cpa-client.py');client=importlib.util.module_from_spec(spec);spec.loader.exec_module(client)
rows=json.loads((a.evidence/'source-records.json').read_text(encoding='utf-8'))['xinghai']
index=[dict(id=r['source_id'],name=r['name'],description=r['content'][:400]) for r in rows if r['source_id'][0] in 'PISL']
references=[dict(source=r['id'],line=line) for r in rows for line in r['content'].splitlines() if re.search(r'\b[PISL]\d{2}\b|[PISL]\d{2}[\u4e00-\u9fff]',line)]
payload=dict(index=index,references=references)
(a.evidence/'semantic-reference-audit-input.json').write_text(json.dumps(payload,ensure_ascii=False,indent=2),encoding='utf-8')
result=client.generate('你是世界书语义校对员。检查references中编号旁的名称、用途、种类是否与index同一实体。不是只查编号存在。不要修改原文，只报确实矛盾；合理简称不算错误。当前已修过蒙托/雷尔蒙特/莉娜/维克/晨星编号、发烟筒/EMP独立物、S35查账改S36、I08速射炮改I04等，不要凭旧印象重复报。只返回JSON {"issues":[{"source":"稳定ID","quote":"原句","target":"编号","problem":"语义冲突","suggestion":"最小修法"}]}。',json.dumps(payload,ensure_ascii=False),max_tokens=12000)
(a.evidence/'semantic-reference-audit-response.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
t=result['text'];data=json.loads(t[t.index('{'):t.rindex('}')+1]);(a.evidence/'semantic-reference-audit-candidates.json').write_text(json.dumps(data,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(dict(references=len(references),issues=len(data['issues']),model=result['reported_model'])),flush=True)
