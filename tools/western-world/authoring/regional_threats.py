"""Anonymous threats use existing owned weapons and learned technique rules."""
BEASTS=[
 [('河堤饥狼','Hungry Riverbank Wolf'),('翻粮獠猪','Granary Tuskboar')],
 [('赤崖猎犬','Redcliff Hunting Hound'),('裂角山羊','Split-Horn Mountain Goat')],
 [('苔背伏蜥','Mossback Ambush Lizard'),('泥喉獠兽','Mudthroat Tuskbeast')],
 [('弃井盲獾','Abandoned-Shaft Blind Badger'),('盐壳岩蜥','Saltcrust Rock Lizard')],
 [('浅湾钩嘴鸟','Shallowbay Hookbill'),('镜苔水獭','Mirror-Moss Otter')],
 [('废墙猎獒','Ruinwall Hunting Mastiff'),('渠底甲蟹','Culvert Armored Crab')],
 [('礁缝长鳗','Reef-Crack Long Eel'),('断帆海猎犬','Broken-Sail Sea Hound')],
 [('饥荒灰豺','Famine Gray Jackal'),('掘根野猪','Root-Digging Boar')],
]
HUMANS=[
 [('失饷步卒','Unpaid Levy Soldier','travel-blade',['blade','halt']),('拦车持矛者','Wagon-Stop Spear Carrier','ash-spear',['spear','brace'])],
 [('离堡弓手','Keep Deserter Archer','shortbow',['bow','bindshot']),('墓道掠取者','Crypt Plunderer','dagger',['knife','feint'])],
 [('林路设网者','Forest-Road Trapper','shortbow',['bow','bindshot','quietstep']),('枯枝术士','Withered-Bough Caster','rootstaff',['thorn','rootbind','bark'])],
 [('废矿索费者','Abandoned-Mine Toll Taker','ash-spear',['spear','halt','brace']),('碎晶掮客的打手','Shard Broker’s Enforcer','focus-staff',['spark','ward','frost'])],
 [('流亡抄术者','Exiled Spell Copyist','focus-staff',['spark','ward','seal']),('债契护卫','Debt-Contract Guard','travel-blade',['blade','riposte','shield'])],
 [('暗渠收货人','Culvert Fence','dagger',['knife','venom','smoketurn']),('旧庭受雇卫士','Hired Old-Court Sentinel','ash-spear',['spear','brace','shield'])],
 [('失帆劫舟者','Sail-Lost Boat Raider','longbow',['bow','longshot','bindshot']),('余火私掠术士','Ember Privateer Caster','focus-staff',['spark','starfire','mirror'])],
 [('逃粮车队护手','Fleeing Grain Convoy Guard','ash-spear',['spear','halt','brace']),('争地雇战者','Land-Dispute Mercenary','travel-blade',['blade','riposte','banner'])],
]

def diversify_threat(op,region,index,skills,items):
 if index<2:
  name,en=BEASTS[region][index]
  # Species changes presentation and habitat; no unimplemented venom, flight,
  # telepathy or magical defenses are claimed as a formal mechanic.
  op['name']=name
  return en
 name,en,weapon,methods=HUMANS[region][index-2]
 op['name']=name;op['weapon_id']='sg-'+weapon
 op['inventory']={op['weapon_id']:1};op['skill_ids']=[];op['skill_ranks']={}
 for material,n in items[op['weapon_id']].get('materials',{}).items():op['inventory'][material]=n*12
 for short in methods:
  skill=skills['sg-'+short]
  if skill['required_rank']>op['rank']:continue
  op['skill_ids'].append(skill['id']);op['skill_ranks'][skill['id']]=min(1+op['rank']//7,3)
  # A narrow learned specialty gives this copyist a real mental response.
  # Other techniques retain ordinary regional mastery and all actual costs.
  if op['id']=='sg-op19' and short=='seal':op['skill_ranks'][skill['id']]=3
  if skill.get('required_tool'):op['inventory'][skill['required_tool']]=1
  for material,n in skill.get('materials',{}).items():op['inventory'][material]=max(op['inventory'].get(material,0),n*8)
 return en
