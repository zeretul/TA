"""Validate provenance, translations, identities and actual effect consumers."""
import argparse,csv,hashlib,json,re
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--source-root',type=Path,required=True);p.add_argument('--evidence',type=Path,required=True);a=p.parse_args()
root=Path(__file__).resolve().parents[1];assets=root/'internal/storypack/assets'
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def write(path,data):path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def references(value,needle):
    if isinstance(value,str):return value==needle
    if isinstance(value,dict):return any(references(x,needle) for x in value.values())
    if isinstance(value,list):return any(references(x,needle) for x in value)
    return False
def field(body,names):
    for name in names:
        m=re.search(r'^[*\-] \*\*'+re.escape(name)+r'\*\*[：:]?\s*(.+)',body,re.M)
        if m:return m.group(1).strip()
    return ''

report={'source_hashes':[],'worlds':[],'limits':['Original manuscripts remain unchanged; player effects are bounded adaptations with individually recorded consumers.','The report verifies data and source links, not a real-model playthrough or a release installation.']}
for row in read(a.evidence/'source-hashes.json'):
    actual=hashlib.sha256((a.source_root/row['path']).read_bytes()).hexdigest()
    assert actual==row['sha256'],row['path']
    report['source_hashes'].append(dict(path=row['path'],sha256=actual,unchanged=True))
records=read(a.evidence/'source-records.json');ledger=[]
for pack_id,prefix in [('lanchuan-qunxia','lanchuan'),('xinghai-689','xinghai')]:
    pack=read(assets/(pack_id+'.json'));en=read(assets/(pack_id+'.en-US.json'));rules=pack['story']['gameplay']['adventure_rules'];quests=read(a.evidence/(prefix+'-quest-catalog.json'))
    assert pack['story']['title']==pack['name']
    assert len(pack['characters'])==60 and len(pack['protagonist_templates'])==3
    assert len({w['id'] for w in pack['world']})==len(pack['world'])
    assert {w['id'] for w in pack['world']}==set(en['world'])
    for w in pack['world']:
        e=en['world'][w['id']]
        assert e['name'] and e['content'] and e['brief_description'],w['id']
        assert not re.search('[\u4e00-\u9fff]',e['content']),w['id']
        if w['id'][-3:-2] in ('s','i'):
            assert not re.search('在游戏机制|底层判定|独立耗材|不同于I|个时辰|15分钟',w['content']),w['id']
    defs={x['id']:x for x in rules['skills']+rules['items']}
    world={x['id']:x for x in pack['world']}
    for r in records[prefix]:
        if r['source_id'][0] not in 'SI':continue
        d=defs[r['id']];is_skill=r['source_id'][0]=='S'
        quest_uses=[q['id'] for q in quests if references(q,r['id'])]
        recipe_uses=[x['id'] for x in rules['recipes'] if references(x,r['id'])]
        if is_skill:
            consumers=['ResolveAdventureAction.learn/train']
            if d['kind'] not in ('social','engineering'):consumers.append('resolveAdventureRound')
            if d['kind']=='repair':consumers.append('ResolveAdventureAction.repair')
            if quest_uses:consumers.append('StoryQuest.choice skill prerequisite')
            if recipe_uses:consumers.append('ResolveAdventureAction.craft')
            status='implemented-adaptation'
            if d['kind'] in ('social','engineering') and not quest_uses and not recipe_uses:status='missing-effect-consumer'
        else:
            consumers=['AdventureActor.inventory']
            if d.get('acquisition')=='market':consumers.append('ResolveAdventureAction.buy/sell')
            if d['kind'] in ('weapon','armor'):consumers+=['ResolveAdventureAction.equip','resolveAdventureRound equipment']
            if d.get('effect'):consumers.append('resolveAdventureRound/'+d['effect'] if d['effect'] in ('revive','compute') else 'ResolveAdventureAction.use/'+d['effect'])
            if d['kind'] in ('machine','ship'):consumers+=['ResolveAdventureAction.deploy/repair','resolveAdventureRound asset']
            if quest_uses:consumers.append('StoryQuest.choice item cost/reward')
            if recipe_uses:consumers.append('ResolveAdventureAction.craft material/output')
            status='npc-owned' if r['id'] in ('lanchuan-i10','lanchuan-i50') else 'implemented-item'
        ledger.append(dict(pack_id=pack_id,source_id=r['source_id'],id=r['id'],name=r['name'],source_effect=field(r['content'],['效果意图'] if is_skill else ['实际用途','用途']),implemented_definition=d,consumers=consumers,quest_uses=quest_uses,recipe_uses=recipe_uses,status=status,player_description=world[r['id']]['content']))
    for c in pack['characters']:
        assert c['source_key']==c['lore_id'] and c['lore_id'] in world
        assert not re.search('作者参考|秘密档案',c['brief'])
    report['worlds'].append(dict(id=pack_id,world_entries=len(pack['world']),characters=len(pack['characters']),origins=len(pack['protagonist_templates']),skills=len(rules['skills']),items=len(rules['items']),recipes=len(rules['recipes']),opponents=len(rules['opponents']),markets=len(rules['markets']),quest_definitions=len(quests),rules_bytes=len(json.dumps(rules,ensure_ascii=False,separators=(',',':')).encode()),english_names=len(en['adventure']),sha256=hashlib.sha256((assets/(pack_id+'.json')).read_bytes()).hexdigest()))
write(a.evidence/'effect-consumer-ledger.json',ledger)
with (a.evidence/'effect-consumer-ledger.csv').open('w',encoding='utf-8-sig',newline='') as f:
    fields=['pack_id','source_id','id','name','status','source_effect','consumers','quest_uses','recipe_uses'];writer=csv.DictWriter(f,fieldnames=fields);writer.writeheader()
    for row in ledger:writer.writerow({k:'; '.join(row[k]) if isinstance(row[k],list) else row[k] for k in fields})
report['effect_entries']=len(ledger);report['missing_consumers']=[r['id'] for r in ledger if r['status']=='missing-effect-consumer'];report['reference_corrections']=len(read(a.evidence/'reference-corrections.json'))
write(a.evidence/'content-verification.json',report)
print(json.dumps(report,ensure_ascii=False,indent=2))
assert not report['missing_consumers'],report['missing_consumers']
