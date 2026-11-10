from pathlib import Path
import json,sys,copy,collections,hashlib
from geography import REGIONS,LOCATIONS,FACTIONS
from mechanics import build_rules,SKILLS,ITEMS,metadata
from affairs import build_affairs
from signature_affairs import build_signature_affairs
from affair_outcomes import apply_outcomes
from additional_methods import add_methods
from regional_threats import diversify_threat
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'deliverable';OUT.mkdir(exist_ok=True)
def dump(path,value):path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n')
rules,names,definitions=build_rules()
people=json.loads((ROOT/'authoring/people.json').read_text()) if (ROOT/'authoring/people.json').exists() else []
if isinstance(people,dict):people=people.get('people',people.get('characters',[]))
quests,q_en=build_affairs(rules)
apply_outcomes(quests,q_en)
signatures,sig_en=build_signature_affairs(rules);quests.extend(signatures);q_en.update(sig_en)
add_methods(quests,q_en,rules)
world=[];wen={};characters=[];cen={}
def entry(id,name,en,kind,brief,ben,body,bodyen,tags=None,load='auto',importance='medium'):
 world.append({'id':id,'name':name,'type':kind,'importance':importance,'tags':tags or [],'keywords':[name,en,id],'load_mode':load,'brief_description':brief,'content':body})
 wen[id]={'name':en,'brief_description':ben,'content':bodyen,'tags':[]}
# A sparse regional backbone plus local routes, with explicit two-way travel.
edges=[]
for region in range(8):
 hub=region*6+1
 for offset,minutes in [(1,30),(2,60),(3,120),(4,180),(5,240)]:edges.append((hub,hub+offset,minutes))
 for a,b,minutes in [(hub+1,hub+2,40),(hub+3,hub+4,90)]:edges.append((a,b,minutes))
for a,b,minutes in [(1,7,720),(1,13,480),(1,19,1440),(1,25,1080),(1,43,720),(7,31,1440),(13,19,1080),(13,25,720),(19,31,1800),(25,31,1440),(25,37,1440),(31,37,2160),(37,43,1440),(43,7,1080)]:edges.append((a,b,minutes))
routes=[];labels={};aliases={};neighbors=collections.defaultdict(list)
for a,b,m in edges:
 for x,y in [(a,b),(b,a)]:
  origin=f'sg-l{x:02d}';dest=f'sg-l{y:02d}';rid=f'sg-route-l{x:02d}-l{y:02d}'
  routes.append({'id':rid,'version':'1.0','origin_location_id':origin,'destination_location_id':dest,'segments':[{'id':rid+'.segment','from_location_id':origin,'to_location_id':dest,'duration_minutes':m,'movement_method_id':'ordinary_low_tier_travel_with_normal_rests','zero_time_connection':False}]});neighbors[x].append((y,m))
for n,(name,en,scene,sen,life,len_,act,aen) in enumerate(LOCATIONS,1):
 region=REGIONS[(n-1)//6];id=f'sg-l{n:02d}';labels[id]=name;aliases[id]=[name,en]
 links='；'.join(f'{LOCATIONS[x-1][0]}约{m//60}小时{m%60}分' for x,m in neighbors[n]);links_en='; '.join(f'{LOCATIONS[x-1][1]}: about {m} minutes' for x,m in neighbors[n])
 body=f'所属地域：{region[0]}。\n\n初到景象：{scene}\n\n此地的生活：{life}\n\n可以展开的事：{act}\n\n相邻道路：{links}。行程是普通陆路的正常用时，包含途中休整。天气、关闭或真实阻碍可能改变可达性；先确认路线，不用叙事瞬移。\n\n{region[2]}此地的人会记住亲身经历，也可能只听见部分消息；未亲见的变化应通过来访、信件或传闻逐渐获知。'
 bodyen=f'Region: {region[1]}.\n\nArrival: {sen}\n\nLife here: {len_}\n\nPossibilities: {aen}\n\nRoads: {links_en}. These are ordinary land journeys with normal rests. Weather, closures and real obstacles can change access; confirm the route rather than narrating instant travel.\n\n{region[3]} Residents remember their own experiences and may hear only part of a story. Distant changes become known through visits, letters or rumors.'
 entry(id,name,en,'location',scene,sen,body,bodyen,[region[0]])
for n,(zh,en,text,ten) in enumerate(REGIONS,1):entry(f'sg-region-{n:02d}',zh,en,'world',text,ten,text+' 日常的吃住、劳作、交往与留居都可持续发展；拒绝一件委托不会停止这片地域。',ten+' Food, work, relationships and settling down can continue over time; declining one request never stops the region.')
for slug,zh,en,brief,ben,body,bodyen in FACTIONS:
 entry('sg-faction-'+slug,zh,en,'faction',brief,ben,brief+'\n\n'+body+'\n\n来往方式：先见当地实际联系人，说明自己愿意承担什么。暂时合作、正式加入、拒绝和退出都可讨论；任何身份都不自动赋予私库、首领权或替成员作决定的权利。',ben+'\n\n'+bodyen+'\n\nMeet a real local contact and say what you are willing to undertake. Temporary cooperation, membership, refusal and departure are all possible. No title grants private stores, leadership or authority over other members.')
# The universal effort pool is an explicit game model, not renamed cultivation.
RULES=[
('open-life','没有规定的王冠','No Required Crown','碎冠诸境从帝国解体后的第七十三年、收穗月开始。古灯、遗产、各家权力和地方生活都可延伸，但没有指定天选者、强制复辟、固定邪恶种族或必达终局。玩家可以独行、加入组织、经商、学艺、恋爱、调查、远行或安家。每个人的同意、知情与实际能力分别处理，不因主角重要而自动臣服。','The story begins in Harvest Month, seventy-three years after imperial dissolution. Beacons, legacies, politics and ordinary life remain open. There is no chosen one, compulsory restoration, evil people or required ending. Travel alone, join others, trade, study, love, investigate or settle. Consent, knowledge and ability remain individual.'),
('effort','身体、导具与魔法','Body, Focus and Magic','战斗使用生命、精力、伤势与真实钱物。精力是体能和集中注意的共同有限预算，不是灵力、内力或可无限索取的神恩。法术、武艺、护持都沿实际已学方法结算，施法须有冻结定义要求的导具与材料；法术攻击为导具协同的个人尺度攻击，不另承诺元素克制、召唤军队、瞬移、变形或复活。需要这些新能力时先据现实能力和场景推演，未有实际规则的效果不能凭文字给数值收益。','Combat uses health, effort, injury and real possessions. Effort is the shared finite budget of exertion and concentration, not spiritual power or unlimited divine favor. Learned arts settle through their definitions, with required foci and reagents. Spell attacks are personal-scale focus-assisted attacks; elemental matchups, summoned armies, teleportation, transformation and resurrection are not promised mechanics. Unsupported effects cannot grant numerical benefits through prose.'),
('growth','道路可以交叉','Paths Can Cross','骑士、法师、游侠、盗贼、祭司与德鲁伊是世人理解经历的称呼，不是不可改的职业。出身给已有技能与装备，之后可以取得习本、工具、师友和时间学习其他道路；成长依真实学习与阅历，不靠每日打卡。外表、族属、信仰和职业称呼不自动附加未定义战力。已提交的消耗、伤势、关系、路程与选择进入同一故事，分支保留各自经历，不把旧奖励反复刷新。','Knight, mage, ranger, rogue, priest and druid describe lives, not immutable classes. Origins supply existing skills and gear; later sources, tools, teachers and time permit other paths. Growth follows real practice and experience, not daily check-ins. Appearance, ancestry, belief and titles grant no undefined power. Costs, injuries, relationships, journeys and choices share one story journal; branches preserve their own history and rewards do not respawn.')]
for slug,zh,en,body,ben in RULES:entry('sg-rule-'+slug,zh,en,'rule',body[:90],ben[:180],body,ben,load='resident',importance='high')
RACES=[('人类','Humans','人类寿命与习俗多样，帝国遗民并非人人怀念帝国。','Human lives and customs vary; not every imperial descendant misses the empire.'),('矮人','Dwarves','矿城矮人有工匠也有诗人、商人和失业者；深炉诸家不是所有矮人的父母。','Dwarves include poets, traders and unemployed workers as well as smiths; the houses speak for neither every dwarf nor every family.'),('精灵','Elves','长寿保留更多记忆，也留下更多误记；精灵不是森林的全知代言人。','Long lives preserve memories and mistakes; elves are not omniscient voices of the forest.'),('半身人','Halflings','半身人的身材不决定胆量或职业，果农和流亡信使各有自己的选择。','Stature determines neither courage nor profession; orchard keepers and exiled couriers choose different lives.'),('兽裔','Beastkin','兽裔有不同外貌与亲族习惯，不因外貌天生暴躁或必须效忠某一族长。','Varied beastkin appearances and kinship customs imply neither innate violence nor obedience to one chief.'),('地精','Goblins','地精经营工具摊、修门锁、做航海记录，不生来邪恶，也不人人爱恶作剧。','Goblins trade tools, repair locks and keep nautical records; neither wickedness nor mischief is innate.'),('龙裔','Dragonkin','龙裔保留鳞纹与家族故事，不默认能飞、吐火或继承远古龙的权柄。','Dragonkin bear scales and family stories without automatic flight, fire-breath or ancient draconic authority.')]
for n,(zh,en,body,ben) in enumerate(RACES,1):entry(f'sg-people-{n}',zh,en,'setting',body,ben,body+' 建角时可在自由背景中选择族属与文化；若未指定，不替玩家赋予特殊血统。',ben+' Choose ancestry and culture in the free-form background; unspecified players receive no invented special lineage.')
factions={f[0]:f for f in FACTIONS};skillmap={s['id']:s for s in rules['skills']};itemmap={i['id']:i for i in rules['items']}
for s in rules['skills']:
 en=names[s['id']];desc=s['description'];ben=s['localizations']['en-US']['description'];bg=s['background'];bgen=s['localizations']['en-US']['background']
 entry(s['id'],s['name'],en,'setting',desc,ben,desc+'\n\n'+bg,ben+'\n\n'+bgen,['学艺'])
for i in rules['items']:
 en=names[i['id']];desc=i['description'];ben=i['localizations']['en-US']['description']
 entry(i['id'],i['name'],en,'item',desc,ben,desc+'\n\n'+i['background'],ben+'\n\n'+i['localizations']['en-US']['background'],['物品'])
for q in quests:entry(q['id'],q['title'],q_en[q['id']]['title'],'event',q['brief'],q_en[q['id']]['brief'],q['brief']+'\n\n可以去实际地点询问，也可以暂时不管。未参与、拒绝、失约或改用自己的办法，都可能留下不同后果；固定办法只是已具备正式结算的几种行动。',q_en[q['id']]['brief']+'\n\nAsk at the actual location or leave the matter alone. Refusal, delay, a missed promise or a different method can have consequences; listed methods are the ones with authored settlement.')
# Physical weapons and magic foci are explicit categories, not inferred at runtime.
weaponclasses={'travel-blade':'sword','thorn-sword':'sword','ash-spear':'spear','shortbow':'bow','longbow':'bow','dagger':'blade','focus-staff':'staff','prayer-mace':'staff','rootstaff':'staff','rune-sabre':'blade','glass-lance':'spear','beacon-edge':'sword'}
for slug,cl in weaponclasses.items():itemmap['sg-'+slug]['weapon_class']=cl
for s in rules['skills']:
 school=next(x[8] for x in SKILLS if 'sg-'+x[0]==s['id'])
 if school in ['arcane','druid'] and s['kind']=='attack':s['required_weapon_class']='staff'
 if s['id'] in ['sg-bow','sg-bindshot','sg-longshot','sg-pathkeeper']:s['required_weapon_class']='bow';s.pop('required_tool',None)
 if s['id'] in ['sg-blade','sg-riposte','sg-oath-edge']:s['required_weapon_class']='sword'
 if s['id']=='sg-spear':s['required_weapon_class']='spear'
 if s['id'] in ['sg-knife','sg-feint','sg-venom']:s['required_weapon_class']='blade'
# Ten real local markets, each with a coherent material and teaching selection.
marketspec=[(1,'渡鸦桥集市','Ravenbridge Market',['martial','social'],['travel-blade','padded','bread','tonic','iron','wood','grain','cloth','herbs','lantern','rope','journal','buckler'],('桥头装卸','Bridge unloading',2,22,12)),(7,'断旗军需摊','Brokenbanner Quartermaster',['martial'],['thorn-sword','ash-spear','mail','thorn-mail','iron','cloth','tonic','broth','buckler'],('营垒搬运','Keep hauling',2,26,14)),(13,'林口药草摊','Verge Herb Stall',['wild'],['shortbow','longbow','arrow','herbs','herb-kit','rope','trail-ration','salve','salt','survey-kit'],('药草分拣','Herb sorting',2,20,8)),(14,'根桥小市','Rootbridge Market',['druid'],['rootstaff','root-mail','seed','resin','wood','focus','herbs','cloth'],('林桥整理','Walkway clearing',2,20,10)),(19,'铜脊货街','Copperback Goods Street',['craft'],['iron','forge-tools','forgeplate','glass-lance','crystal','silver-wire','glass','charcoal','leather-strip','spirit'],('矿城卸料','Ore unloading',2,28,16)),(23,'冷锤柜台','Coldhammer Counter',['craft'],['travel-blade','dagger','mail','rune-sabre','forge-tools','iron','wood','leather-strip','cloth'],('工坊备料修件','Workshop supplied repair work',2,32,12)),(25,'镜湖学市','Mirrorlake Study Market',['arcane','social'],['focus-staff','focus','rune-coat','crystal','charcoal','silver-wire','glass','journal','wax','beacon-edge'],('抄本整理','Copy sorting',2,22,8)),(34,'黑蜡换物铺','Blackwax Exchange',['shadow'],['dagger','leather','courtguard','lockpicks','smoke','venom-paste','spore-flask','lantern','rope'],('旧物清点','Secondhand inventory',2,24,10)),(37,'雾湾海市','Mistbay Dock Market',['wild','craft'],['salt','arrow','rope','lantern','spirit','grain','bread','fire-flask','focus','cloth'],('港口理货','Dock sorting',2,26,14)),(48,'修院药房','Hospice Dispensary',['faith'],['prayer-mace','prayer-cord','cloth','herbs','salt','spirit','wax','tonic','salve','greater-tonic','rescue-dose','rest-draught','broth'],('清洗药布','Dressing laundry',2,20,8))]
for loc,zh,en,schools,stock,work in marketspec:
 ids=['sg-'+x for x in stock]
 ids+=['sg-manual-'+s[0] for s in SKILLS if s[8] in schools]
 rules['markets'].append({'id':f'sg-market-{loc:02d}','name':zh,'location':LOCATIONS[loc-1][0],'location_aliases':[f'sg-l{loc:02d}',LOCATIONS[loc-1][1]],'items':ids,'work_service':{'name':work[0],'hours':work[2],'pay':work[3],'energy_cost':work[4]}});names[f'sg-market-{loc:02d}']=en
for market in rules['markets']:
 if market['id']=='sg-market-23':market['work_service'].update(required_skill='sg-forge',skill_rank=1)
rules['markets'][0]['items'] += ['sg-manual-forge','sg-forge-tools','sg-herb-kit']
# Ensure every ordinary priced item has a lawful seller, without granting unseen stock.
stocked={id for m in rules['markets'] for id in m['items']}
missing_stock=[i['id'] for i in rules['items'] if i['acquisition']=='market' and i['id'] not in stocked]
assert not missing_stock, 'priced items need a specific local seller: '+str(missing_stock)

def actor(rank,skills,inventory,currency=30):
 return {'family':'fantasy','rank':rank,'experience':0,'health':100+(rank-1)*20,'max_health':100+(rank-1)*20,'energy':100+(rank-1)*10,'max_energy':100+(rank-1)*10,'injury':0,'currency':currency,'skills':skills,'training':{},'inventory':inventory,'equipment':{},'assets':{}}
# Final people file supplies rich canon; flexible key aliases only ease authoring.
for p in people:
 loc=int(p['location']);id=p['id'];name=p['name'];en=p['en_name'];fac=p.get('faction','');fa=factions.get(fac);rank=int(p['rank'])
 def val(k,en=False):return str(p.get(('en_'+k) if en else k,''))
 rel=p.get('relationships',[]);rt='\n'.join(f"{x['target']}：{x['text']}" for x in rel);re='\n'.join(f"{x['target']}: {x['en_text']}" for x in rel)
 body='\n\n'.join([f"身份：{val('role')}；{p.get('race','')}，{p['age']}岁。",f"外貌：{val('appearance')}",f"言谈：{val('voice')}",f"眼前所求：{val('goal')}",f"可逐步得知：{val('public')}",f"作者背景，需实际获知途径：{val('private')}",f"接触条件：{p.get('access','在日常地点先说明来意，不保证随时有空。')}",f"关系：\n{rt}"])
 bodyen='\n\n'.join([f"Role: {val('role',True)}; {p.get('en_race','')}, age {p['age']}.",f"Appearance: {val('appearance',True)}",f"Speech: {val('voice',True)}",f"Current aim: {val('goal',True)}",f"Discoverable background: {val('public',True)}",f"Author background, requiring an actual discovery source: {val('private',True)}",f"Contact: {p.get('en_access','Ask at the ordinary location; availability and willingness are not guaranteed.')}",f"Relationships:\n{re}"])
 entry(id,name,en,'character',val('public'),val('public',True),body,bodyen,[LOCATIONS[loc-1][0]])
 row={'source_key':id,'name':name,'aliases':[],'country':REGIONS[(loc-1)//6][0],'role':val('role'),'topics':[REGIONS[(loc-1)//6][0]],'initial_place_candidate':f'sg-l{loc:02d}','initial_place_label':LOCATIONS[loc-1][0],'region_tokens':[LOCATIONS[loc-1][0]],'level':'重要' if rank>=15 else '普通','brief':val('public'),'lore_id':id,'contact_gate':p.get('access','在日常地点说明来意，本人可忙碌或拒绝。'),'age':p['age'],'gender':p['gender'],'speech_style':val('voice'),'initial_stage':f'历练{rank}级'}
 if fa:row.update(faction_id='sg-faction-'+fac,faction_name=fa[1])
 if id in ['sg-p01','sg-p07','sg-p20','sg-p55','sg-p44','sg-p14','sg-p38','sg-p34','sg-p48','sg-p43','sg-p31','sg-p60']:row['membership_admission']=['formal','temporary']
 
 if row.get('membership_admission'):
  world[-1]['content']+='\n\n公开职责：本人可受理并决定该组织在本地区的正式与临时成员申请；具体条件需当面说明。'
  wen[id]['content']+='\n\nPublic responsibility: this person is authorized to decide local formal and temporary membership applications; specific terms must be discussed in person.'
 characters.append(row);cen[id]={'name':en,'brief':val('public',True),'role':val('role',True),'initial_place_label':LOCATIONS[loc-1][1],'faction_name':fa[2] if fa else ''}
 tags=p.get('skills',['social']);school=tags[0] if tags else 'social';school='wild' if school=='druid' else school
 known={'sg-'+s[0]:min(1+rank//7,5) for s in SKILLS if s[8] in tags and s[6]<=rank and (s[6]<=3 or rank>=15)}
 if not known:known={'sg-parley':1}
 inv={'sg-padded':1,'sg-bread':3,'sg-tonic':2}
 for skill in (skillmap[k] for k in known):
  if skill.get('required_tool'):inv[skill['required_tool']]=1
  for reagent in skill.get('materials',{}):inv[reagent]=10
 classes={skillmap[k].get('required_weapon_class') for k in known}
 # Equip a real implement for the person's practiced combat path; mixed
 # knowledge never silently changes a weapon during a battle.
 w='focus-staff' if 'staff' in classes else 'shortbow' if 'bow' in classes else 'travel-blade' if 'sword' in classes else 'dagger' if 'blade' in classes else 'prayer-mace' if 'faith' in tags else 'travel-blade'
 signature_loadout={'sg-p07':('thorn-sword','thorn-mail'),'sg-p56':('focus-staff','rune-coat')}.get(id)
 if signature_loadout:w=signature_loadout[0]
 armor='sg-'+signature_loadout[1] if signature_loadout else 'sg-padded'
 inv[armor]=1
 inv['sg-'+w]=1
 for reagent in itemmap['sg-'+w].get('materials',{}):inv[reagent]=12
 for sk in known:inv['sg-manual-'+sk[3:]]=1
 a=actor(rank,known,inv,currency=20+rank*8);a['equipment']={'person:weapon':'sg-'+w,'person:armor':armor}
 train=id in ['sg-p08','sg-p23','sg-p27','sg-p48'];sk=next(iter(known));rules['npcs'].append({'character_source_key':id,'actor':a,'plan':{'action':'train' if train else 'none','skill_id':sk if train else '', 'training_percent':20 if train else 0,'rank_limit':min(30,rank+2) if train else rank}})
# Named combatants bind existing people's actual state; these rows neither
# spawn them nor make an unspoken challenge, consent or hostility canonical.
for person in people:
 if int(person['id'][-2:]) not in [5,7,8,10,13,15,21,23,26,28,30,35,38,41,47,52,54,55,56]:continue
 npc=next(n for n in rules['npcs'] if n['character_source_key']==person['id']);a=npc['actor'];loc=person['location'];rank=a['rank'];id='sg-encounter-'+person['id'][3:]
 rules['opponents'].append({'id':id,'name':person['name'],'scale':'person','character_source_key':person['id'],'requires_presence':True,'location':LOCATIONS[loc-1][0],'location_aliases':[f'sg-l{loc:02d}',LOCATIONS[loc-1][1]],'rank':rank,'health':a['max_health'],'energy':a['max_energy'],'attack':10+rank*3,'defense':rank+3,'lethal':False,'reward_currency':0,'reward_experience':rank*2})
 names[id]=person['en_name']
# Four clearly bounded encounter definitions per region; challenge is not automatic hostility.
beasts=[('拦路饥狼','Hungry Road Wolf'),('沼地獠兽','Marsh Tuskbeast'),('逃散劫掠者','Straggling Raider'),('受雇恶棍','Hired Ruffian')]
for reg in range(8):
 for j,(zh,en) in enumerate(beasts):
  n=reg*4+j+1;rank=1+reg*2+j;loc=reg*6+1+j;sk='sg-blade' if j>=2 else 'sg-knife';weapon='sg-travel-blade' if j>=2 else 'sg-dagger'
  opp={'id':f'sg-op{n:02d}','name':REGIONS[reg][0]+'·'+zh,'scale':'person','location':LOCATIONS[loc-1][0],'location_aliases':[f'sg-l{loc:02d}',LOCATIONS[loc-1][1]],'health':60+rank*16,'attack':7+rank*2,'defense':rank,'rank':rank,'energy':60+rank*10,'reward_currency':6+rank*2 if j>=2 else 0,'reward_experience':12+rank*3,'lethal':j!=2,'weapon_id':weapon,'inventory':{weapon:1},'skill_ids':[sk]}
  # Animals use innate attacks, not a hidden dagger in their inventory.
  if j<2:opp.pop('weapon_id');opp['inventory']={};opp['skill_ids']=[]
  en=diversify_threat(opp,reg,j,skillmap,itemmap)
  rules['opponents'].append(opp);names[opp['id']]=en
# A safe local sparring opponent is an offered exercise, never an unsolicited attack.
for id,zh,en,loc,rank,hp,attack,defense in [('sg-crypt-warden','旧誓铁壳','Old Oath Iron Shell',12,4,130,13,3),('sg-drowned-keeper','沉库守卫','Drowned Vault Keeper',35,6,170,17,4)]:
 rules['opponents'].append({'id':id,'name':zh,'scale':'person','location':LOCATIONS[loc-1][0],'location_aliases':[f'sg-l{loc:02d}',LOCATIONS[loc-1][1]],'rank':rank,'health':hp,'attack':attack,'defense':defense,'energy':60,'mindless':True,'lethal':True,'reward_currency':0,'reward_experience':rank*6});names[id]=en
rules['opponents'].insert(0,{'id':'sg-practice','name':'桥工的练习木靶','scale':'person','location':'渡鸦桥镇','location_aliases':['sg-l01','Ravenbridge'],'health':55,'attack':1,'defense':1,'rank':1,'energy':0,'mindless':True,'reward_currency':0,'reward_experience':6,'lethal':False});names['sg-practice']='Bridge Yard Practice Target'
starts=[('road','桥镇行路人','Bridge Traveler',1,['blade','parley'],['travel-blade','padded'],'你靠短工与行路见闻来到渡鸦桥，不欠任何组织一个尚未答应的誓言。','You reached Ravenbridge through odd jobs and travel, owing no organization an unspoken oath.'),('learner','借灯学徒','Borrowed Lamp Apprentice',25,['spark','ward','lore'],['focus-staff','focus','padded'],'你替镜湖学社整理抄本，已学两种基础术法，正式入会仍是日后可谈的事。','You sort copies for a lakeside school and know two basic arts; formal membership remains a future choice.'),('scout','离乡寻径者','Displaced Pathfinder',13,['trail','bow','herbwise'],['shortbow','herb-kit','padded'],'你沿林缘替同行者找路，留下还是继续寻亲，都还没有答案。','You guide fellow travelers along the forest edge, undecided between settling and continuing a family search.'),('smith','冷锤帮工','Coldhammer Helper',23,['forge','fieldrepair','blade'],['forge-tools','travel-blade','padded'],'你会做可靠的小修，工坊师傅愿意继续教，但没有替你决定一生只打铁。','You make dependable small repairs; the master may teach more without deciding you must smith forever.'),('carer','修院守夜人','Hospice Night Keeper',48,['firstaid','prayer','parley'],['prayer-cord','prayer-mace','herb-kit','padded'],'你在修院照料旅人，信仰、来历与远行理由可以由你自己说明。','You care for travelers at the hospice; your beliefs, background and reasons for leaving are your own.'),('grove','根桥拾枝客','Rootbridge Gatherer',14,['greenhand','thorn','herbwise'],['rootstaff','focus','herb-kit','padded'],'你懂得取枝前问主人，也学过有限自然法术，不因此成了整片森林的使者。','You ask before taking branches and know limited natural arts, without claiming to speak for the forest.'),('runner','黑蜡跑腿人','Blackwax Runner',34,['listen','knife','parley'],['dagger','padded'],'你靠记路和可靠传话吃饭，见过灰色买卖，却仍可选择不接伤人的事。','You earn through routes and reliable messages, familiar with gray trade but free to refuse harmful work.')]
templates=[];origins=[];ten={};origin_en={}
for slug,zh,en,loc,known,gear,profile,pen in starts:
 id='sg-start-'+slug;known=list(dict.fromkeys(known+['parley']));skills={'sg-'+s:1 for s in known};inv=copy.deepcopy(rules['starting_items']);inv.pop('sg-travel-blade',None)
 for g in gear:inv['sg-'+g]=1
 for sk in known:inv['sg-manual-'+sk]=1
 for sk in skills:
  for reagent in skillmap[sk].get('materials',{}):inv[reagent]=6
 for g in gear:
  for reagent in itemmap['sg-'+g].get('materials',{}):inv[reagent]=6
 inv['sg-cloth']=max(2,inv.get('sg-cloth',0));inv['sg-herbs']=max(2,inv.get('sg-herbs',0))
 a=actor(1,skills,inv,24);wep=next((g for g in gear if itemmap['sg-'+g]['kind']=='weapon'),None)
 if wep:a['equipment']['person:weapon']='sg-'+wep
 a['equipment']['person:armor']='sg-padded'
 # Starts may know a rank-2 practical trade through real prior work, never a
 # higher numerical rank. Make entry methods reachable at rank 1 in the catalog.
 for sk in known:skillmap['sg-'+sk]['required_rank']=1
 origin=LOCATIONS[loc-1][0]+'。'+profile
 opening=LOCATIONS[loc-1][2]+'\n\n'+profile+'你带着已有的工具与少量银铢，眼前有人忙着日常生计，也有还没谈妥的事。先找一份工作、问一段路、拜访某个人，或坐下来吃点东西，都由你决定。'
 state={'姓名':'旅人','基本身份':zh,'性格与背景':profile,'当前处境':origin,'历练状态':a,'历练等级':1,'生命':100,'生命上限':100,'精力':100,'精力上限':100,'伤势程度':0,'钱财':24}
 templates.append({'id':id,'name':zh,'description':profile,'character_name':'旅人','profile':profile,'origin':origin,'opening_text':opening,'state':state});origins.append({'id':id,'name':zh,'description':'你眼下落脚在'+LOCATIONS[loc-1][0]+'。'+profile,'template_id':id,'default':slug=='road'});ten[id]={'name':en,'description':pen};origin_en[id]={'name':en,'description':'You are currently based in '+LOCATIONS[loc-1][1]+'. '+pen}
for recipe in rules['recipes']:
 recipe['required_tool']='sg-forge-tools' if recipe['skill_id'] in ['sg-forge','sg-masterforge','sg-glasswork'] else 'sg-herb-kit' if recipe['skill_id'] in ['sg-alchemy','sg-herbwise'] else 'sg-focus' if recipe['skill_id'] in ['sg-runes','sg-beaconwork'] else 'sg-forge-tools'
for group in ['skills','items','recipes']:
 for definition in rules[group]:definition.pop('localizations',None)
pack={'version':1,'id':'sui-guan-realms','name':'碎冠诸境','description':'旧帝国的王冠已经碎裂，烬河的磨坊仍在转。沿渡鸦桥、古灯塔与无名王庭，你可以学艺、谋生、结伴或独行，在会记得你的诸境中选择自己的生活。','tags':['原创西幻','开放世界','多种族'],'content_version':1,'recommended_planning_template_id':'episodic-emergent','world':world,'characters':characters,'protagonist_templates':templates,'creation':{'age_range':[20,32],'genders':[{'id':'male','name':'男','default':True},{'id':'female','name':'女'},{'id':'unspecified','name':'不作说明'}],'origins':origins,'state_basis_templates':[x['id'] for x in templates],'affiliations':[{'id':'none','name':'由经历决定','description':'无自动加入；日后可独行、合作或真实申请加入组织。','kind':'none','default':True}],'locations':[{'id':'auto','name':'随所选出身','description':'从已有生活所在处开始。','default':True}],'effect_notes':{'origins':'narrative','affiliations':'narrative','locations':'narrative'}},'topic_terms':{'学艺':['骑士','法师','游侠','盗贼','祭司','德鲁伊','习本','训练'],'生计':['工作','工钱','制作','摆摊','采药'],'旧帝国':['王庭','继承','古灯','档案']}}
base=templates[0];pack['story']={'title':'碎冠诸境：烬河来客','origin':base['origin'],'protagonist_name':'旅人','protagonist':base['profile'],'opening_text':base['opening_text'],'reply_target_chars':1600,'choice_count':3,'module_refs':{'event_package_ids':['default'],'actor_state_id':'fantasy-state-v1','rule_system_id':'fantasy-rules-v1','image_preset_id':'realistic'},'gameplay':{'status_template':'fantasy-state-v1','numeric_scale':'fantasy','growth':'fantasy','combat':'fantasy','communication':'letter','letter_delivery_days':2,'opening':'scene','confirmed':True,'adventure_rules':rules},'narrative_requirements':'保持原创西幻的具体生活与行动自由。出身不锁职业，种族不决定善恶，NPC有各自生计与立场。已知、传闻、作者背景分开；不要替玩家加入组织、立誓、恋爱或接受终局。魔法和武艺只从已学能力、持有工具、正式资源与本局事实推演。事件并非每日打卡；不参加也允许世界按实际因果发展。'}
local={'name':'Shattered Crown Realms','description':'The empire’s crown is broken, but Ashriver’s mills still turn. Across Ravenbridge, ancient beacons and the Nameless Court, learn, work, travel together or alone, and choose a life in realms that remember.','tags':['Original fantasy','Open world','Many peoples'],'title':'Shattered Crown Realms: An Ashriver Arrival','protagonist':'Traveler','world':wen,'characters':cen,'templates':ten,'creation':{'origins':origin_en,'affiliations':{'none':{'name':'Determined by experience','description':'No automatic enlistment. Travel alone, cooperate or actually apply to an organization later.'}},'locations':{'auto':{'name':'Selected origin','description':'Start where the selected life is already taking place.'}},'genders':{'male':{'name':'Male'},'female':{'name':'Female'},'unspecified':{'name':'Unspecified'}}},'initial_state':{'currency_label':'Silver marks','items':{i['name']:names[i['id']] for i in rules['items']},'techniques':{i['name']:names[i['id']] for i in rules['skills']}},'adventure':names,'definitions':definitions}
dump(OUT/'storypack/sui-guan-realms.json',pack);dump(OUT/'storypack/sui-guan-realms.en-US.json',local);dump(OUT/'quest/sui-guan.json',quests);dump(OUT/'quest/sui-guan.en-US.json',q_en);dump(OUT/'routes/sui-guan-routes.json',{'routes':routes,'labels':labels,'aliases':aliases})
report={'world_entries':len(world),'world_types':dict(collections.Counter(x['type'] for x in world)),'characters':len(characters),'skills':len(rules['skills']),'items':len(rules['items']),'recipes':len(rules['recipes']),'opponents':len(rules['opponents']),'npcs':len(rules['npcs']),'markets':len(rules['markets']),'starts':len(templates),'routes':len(routes),'affairs':len(quests),'choices':sum(len(q['choices']) for q in quests),'echo_followups':sum('requires_echo' in q for q in quests),'localized_world_entries':len(wen),'localized_characters':len(cen),'runtime_art_entries':len(json.loads((OUT/'art/manifest-delta.json').read_text())['entries']) if (OUT/'art/manifest-delta.json').exists() else 0,'reviewed_original_portraits':sum(1 for line in (ROOT/'art/npc-art-records.jsonl').read_text().splitlines() if line.strip() and json.loads(line).get('review_state')=='visual_review_passed') if (ROOT/'art/npc-art-records.jsonl').exists() else 0,'verification':'Authoring assembly only; not runtime verification.'}
dump(ROOT/'evidence/content_counts.json',report);print(json.dumps(report,ensure_ascii=False,indent=2))
