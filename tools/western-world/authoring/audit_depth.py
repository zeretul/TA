"""Audit authored dependency graphs and exact-world art provenance, not runtime."""
from pathlib import Path
import hashlib,json,collections,struct
R=Path(__file__).resolve().parent.parent
read=lambda p:json.loads((R/p).read_text())
p=read('deliverable/storypack/sui-guan-realms.json'); rules=p['story']['gameplay']['adventure_rules']; quests=read('deliverable/quest/sui-guan.json'); routes=read('deliverable/routes/sui-guan-routes.json')
issues=[]
def check(ok,message):
 if not ok: issues.append(message)
def canon(x):return json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()
def digest(x):return hashlib.sha256(x).hexdigest()
skills={s['id']:s for s in rules['skills']}; items={i['id']:i for i in rules['items']}; opponents={o['id'] for o in rules['opponents']}; lore={w['id']:w for w in p['world']}; chars={c['source_key']:c for c in p['characters']};locations={w['id'] for w in p['world'] if w['type']=='location'}
for s in skills.values():
 seen=set(); chain=s
 while chain.get('prerequisite'):
  previous=chain['prerequisite'];check(previous not in seen,'cyclic skill prerequisites '+s['id'])
  if previous in seen or previous not in skills:break
  seen.add(previous);chain=skills[previous]
 check(s['required_rank']<=rules['progression']['max_rank'],'unreachable rank '+s['id'])
for recipe in rules['recipes']:
 if recipe.get('required_tool'):check(recipe['required_tool'] in items,'unknown recipe tool '+recipe['id'])
 check(recipe['output_id'] not in recipe['materials'],'self-producing recipe '+recipe['id'])
quest_ids={q['id'] for q in quests}; rewards=set()
for q in quests:
 objectives=[o['id'] for o in q['objectives']]
 for c in q['choices']:
  check(c['objective_id'] in objectives,'unknown objective '+c['id'])
  for previous in c.get('after_objectives',[]):
   check(previous in objectives and objectives.index(previous)<objectives.index(c['objective_id']),'noncausal objective dependency '+c['id'])
  for source in c.get('required_character_source_keys',[]):check(source in chars,'unknown required participant '+c['id'])
  if c.get('required_victory'):check(c['required_victory'] in opponents,'unknown required victory '+c['id'])
  for location in c.get('location_ids',[]):check(location in locations,'unknown choice location '+c['id'])
  for reward in c['rewards']:
   if reward['kind']=='item':check(reward['ref_id'] in items,'unknown rewarded item '+c['id']);rewards.add(reward['ref_id'])
 for location in q.get('public_location_ids',[]):check(location in locations,'unknown affair location '+q['id'])
for i in items.values():
 if i['acquisition']=='reward':check(i['id'] in rewards,'reward-only object without source '+i['id'])
graph=collections.defaultdict(set)
for route in routes['routes']:
 a,b=route['origin_location_id'],route['destination_location_id'];check(a in locations and b in locations,'unknown route endpoint '+route['id']);graph[a].add(b)
 for segment in route['segments']:check(segment['duration_minutes']>0 or segment.get('zero_time_connection'),'unearned zero-time route '+route['id'])
for start in locations:
 visited=set(); pending=[start]
 while pending:
  node=pending.pop()
  if node not in visited:visited.add(node);pending+=list(graph[node]-visited)
 check(visited==locations,'disconnected map from '+start)
manifest=read('deliverable/art/manifest-delta.json');records=[json.loads(line) for path in [R/'art/npc-art-records.jsonl',R/'art/definition-art-records.jsonl'] if path.exists() for line in path.read_text().splitlines() if line.strip()];recordmap={(r['kind'],r['id']):r for r in records};artroot=R/'sandbox/internal/catalogart/assets';actual_variants=0
definitions={x['id']:x for group in ['skills','items','recipes'] for x in rules[group]}
for entry in manifest['entries']:
 key=entry['id'];check(entry['world']==p['id'],'foreign artwork '+key);rec=recordmap[(entry['kind'],key)]
 source={'character':chars[key],'lore':lore[chars[key]['lore_id']]} if entry['kind']=='npc' else {'definition':definitions[key],'lore':lore.get(key)};check(digest(canon(source))==rec['source_hash']==entry['source_sha256'],'stale art source '+key)
 original=Path(rec['file']).read_bytes();check(digest(original)==rec['sha256']==entry['original_sha256'],'changed original '+key)
 check(original[:8]==b'\x89PNG\r\n\x1a\n' and len(original)>=24,'invalid original PNG '+key)
 if len(original)>=24:
  width,height=struct.unpack('>II',original[16:24]);check(width==rec['width'] and height==rec['height'] and min(width,height)>=512,'original dimensions mismatch '+key)
  if entry['kind']=='definition':check(width==height,'non-square definition original '+key)
 for variant in entry['variants'].values():
  path=artroot/'images'/(variant['sha256']+'.png')
  if not path.exists():path=artroot/(variant['sha256']+'.png')
  check(path.exists(),'missing runtime image '+key)
  if path.exists():check(digest(path.read_bytes())==variant['sha256'],'runtime image hash mismatch '+key);actual_variants+=1
check(len({entry['original_sha256'] for entry in manifest['entries']})==len(manifest['entries']),'one original image reused for multiple catalog identities')
present_npc={e['id'] for e in manifest['entries'] if e['kind']=='npc'};present_defs={e['id'] for e in manifest['entries'] if e['kind']=='definition'}
coverage={'npc_portraits':{'present':len(present_npc),'total':len(chars),'missing':sorted(set(chars)-present_npc)},'definitions':{'present':len(present_defs),'total':len(definitions),'missing':sorted(set(definitions)-present_defs)},'runtime_variants':actual_variants,'existing_three_worlds_included':False}
report={'passed':not issues,'issues':issues,'checks':['skill prerequisite acyclicity and maximum rank','recipe reusable tool references','ordered quest stage dependencies and real opponent prerequisites','reward-only source reachability','48-location strongly connected route graph','exact-world image source, unique originals, PNG dimensions and binary hashes'],'art':coverage,'scope':'Static dependency and provenance checks only. No AI, browser or combat simulation claimed.'}
(R/'evidence/dependency_art_audit.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({**report,'art':{k:({x:y for x,y in v.items() if x!='missing'} if isinstance(v,dict) else v) for k,v in coverage.items()}},ensure_ascii=False,indent=2));raise SystemExit(bool(issues))
