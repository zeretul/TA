from pathlib import Path
import json,collections,re
r=Path(__file__).resolve().parent.parent
p=json.loads((r/'deliverable/storypack/sui-guan-realms.json').read_text());en=json.loads((r/'deliverable/storypack/sui-guan-realms.en-US.json').read_text());rules=p['story']['gameplay']['adventure_rules'];issues=[]
def check(ok,msg):
 if not ok:issues.append(msg)
for group,arr,key in [('world',p['world'],'id'),('characters',p['characters'],'source_key'),('skills',rules['skills'],'id'),('items',rules['items'],'id'),('recipes',rules['recipes'],'id'),('opponents',rules['opponents'],'id')]:
 ids=[x[key] for x in arr];check(len(ids)==len(set(ids)),f'duplicate {group} IDs')
world={x['id']:x for x in p['world']};chars={x['source_key']:x for x in p['characters']};skills={x['id']:x for x in rules['skills']};items={x['id']:x for x in rules['items']};recipes={x['id']:x for x in rules['recipes']}
for id,c in chars.items():check(c['lore_id'] in world,f'missing character lore {id}');check(c['initial_place_candidate'] in world,f'missing place {id}')
for s in skills.values():
 check(s.get('required_item','') in items,'missing learning source '+s['id'])
 if s.get('required_tool'):check(s['required_tool'] in items,'missing tool '+s['id'])
 if s.get('prerequisite'):check(s['prerequisite'] in skills,'missing prerequisite '+s['id'])
 for id in s.get('materials',{}):check(id in items,'missing reagent '+id)
for x in recipes.values():
 check(x['skill_id'] in skills,'missing crafting skill '+x['id']);check(x['output_id'] in items,'missing output '+x['id'])
 for id in x['materials']:check(id in items,'missing ingredient '+id)
stock={id for m in rules['markets'] for id in m['items']}
for id,i in items.items():
 if i['acquisition']=='market':check(id in stock,'no lawful market for '+id)
for a in [x['state']['历练状态'] for x in p['protagonist_templates']]+[x['actor'] for x in rules['npcs']]:
 for id,level in a['skills'].items():
  check(id in skills,'unknown learned '+id)
  if id in skills:
   check(skills[id]['required_rank']<=a['rank'],'unqualified learned '+id)
   if skills[id].get('prerequisite'):check(a['skills'].get(skills[id]['prerequisite'],0)>0,'unlearned foundation '+id)
 for id in a['inventory']:check(id in items,'unknown owned '+id)
 for id in a['equipment'].values():check(a['inventory'].get(id,0)>0,'unowned equipment '+id)
for m in rules['markets']:
 for f in ['hours','pay','energy_cost']:check(type(m['work_service'][f]) is int,'invalid work field '+m['id']+':'+f)
for q in json.loads((r/'deliverable/quest/sui-guan.json').read_text()):
 for c in q['choices']:
  check(c['skill_id'] in skills,'unknown affair skill '+q['id'])
  check(c.get('required_skill')==c['skill_id'],'ungated affair skill '+c['id'])
  for reward in c['rewards']:
   if reward['kind']=='currency':check(reward['ref_id']==rules['currency'],'wrong reward currency '+c['id'])
  if c.get('use_skill_id'):check(skills[c['use_skill_id']]['kind'] in ['engineering','repair','social','pilot'],'combat quest method '+c['id'])
  for item in c.get('required_items',{}):check(item in items,'missing quest tool '+c['id'])
  for id in c.get('item_costs',{}):check(id in items,'unknown cost '+id)
  check(c['success_reaction']['character_source_key'] in chars,'unknown reaction person '+q['id'])
for e in p['world']:
 check(e['id'] in en['world'],'untranslated world '+e['id'])
 if e['id'] in en['world']:check(not re.search('[\u4e00-\u9fff]',en['world'][e['id']]['content']),'Chinese leaked to EN '+e['id'])
report={'passed':not issues,'issues':issues,'checks':'IDs, cross-references, lawful sources, start/NPC qualification, EN world coverage. Static only.'}
(r/'evidence/static_validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False,indent=2));raise SystemExit(bool(issues))
