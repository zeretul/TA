"""Build the two reviewed worlds from immutable, user-supplied manuscripts.

Source identities are chapter/category plus the author's number, never names or
file positions. Candidate translations are merged only by these stable IDs.
"""
import argparse, hashlib, json, re
from pathlib import Path
from astra_npc_catalog import build_npc_catalog

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--source-root', type=Path, required=True)
parser.add_argument('--evidence', type=Path, required=True)
args = parser.parse_args()
OUT = args.evidence
OUT.mkdir(parents=True, exist_ok=True)

def write(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')

def plain(s):
    return re.sub(r'[*`#>]', '', s).strip()

def field(body, *names):
    for name in names:
        match = re.search(r'^[*\-] \*\*'+re.escape(name)+r'\*\*[：:]?\s*(.+)', body, re.M)
        if match: return plain(match.group(1)).lstrip('：:')
    return ''

def chapter(text, start, end):
    return text[text.index(start):text.index(end)]

def entries(text, pattern):
    matches = list(re.finditer(pattern, text, re.M))
    return [(m, text[m.end():matches[i+1].start() if i+1<len(matches) else len(text)].strip().rstrip('-').strip()) for i,m in enumerate(matches)]

def clean_name(name):
    return re.split(r'[（〔【｜]', name)[0].strip('《》 ：\r\n')

def extract(prefix, text):
    records=[]
    def add(code, name, body, region=''):
        records.append(dict(id=f'{prefix}-{code.lower()}',source_id=code,name=clean_name(name),title=name,content=body,region=region))
    if prefix=='xinghai':
        for m,body in entries(text, r'^#{3,4} ([LOPAEQSID]\d{2})[ \t]+([^\n]+)'):
            code,name=m.groups(); body=re.split(r'^#{1,3} ', body, maxsplit=1, flags=re.M)[0].strip().rstrip('-').strip()
            add(code,name,body)
        for i,(m,body) in enumerate(entries(chapter(text,'## 第二章','## 第三章'),r'^### 区域[^：]+：([^\n]+)'),1):
            name=m.group(1); region=clean_name(name)
            add(f'R{i:02}',name,body.split('####')[0].strip())
            for r in records:
                if r['source_id'].startswith('L') and (i-1)*5<int(r['source_id'][1:])<=i*5:r['region']=region
        add('W01','星海历689：世界与时代',chapter(text,'## 第一章','## 第二章'))
        origin_text=chapter(text,'## 第四章','## 第五章')
        for i,(m,b) in enumerate(entries(origin_text,r'^### 4\.[123] ([^\n]+)'),1):
            add(f'C{i:02}',m.group(1),b.split('### 4.4')[0].strip())
        # The author explicitly separates these secret facts from the public matrix.
        secrets=re.search(r'(\|\s*P\d{2}\s*\|[^\n]+)',text)
        secret_rows=[l for l in text.splitlines() if re.match(r'\|\s*P\d{2}\s*\|',l)]
        if secret_rows:add('W02','作者参考：人物秘密与揭示条件','\n'.join(secret_rows))
    else:
        blocks=[('L','第二章','第三章',r'^### (\d+)\. ([^\n]+)'),('O','第三章','第四章',r'^### (\d+)\. ([^\n]+)'),('P','第五章','第六章',r'^### (\d+)\. ([^\n]+)'),('A','第六章','第七章',r'^### A(\d+)《([^\n]+)'),('E','第七章','第八章',r'^## 强敌 B(\d+)：([^\n]+)'),('Q','第八章','第九章',r'^## P(\d+)《([^\n]+)'),('S','第九章','第十章',r'^### (\d+)\. ([^\n]+)'),('I','第十章','第十一章',r'^### I(\d+) ([^\n]+)'),('D','第十一章','第十二章',r'^## (\d+)\. ([^\n]+)')]
        for code,start,end,pattern in blocks:
            block=chapter(text,'# '+start,'# '+end)
            for m,body in entries(block,pattern):
                n,name=m.groups(); body=re.split(r'^#{1,2} ',body,maxsplit=1,flags=re.M)[0].strip().rstrip('-').strip()
                region=''
                if code=='P':
                    heads=list(re.finditer(r'^## ([^\n（]+)',block[:m.start()],re.M));region=heads[-1].group(1) if heads else ''
                if code=='L':region=field(body,'所属地域').split('（')[0]
                add(f'{code}{int(n):02}',name,body,region)
        trades=chapter(text,'## 第二部分　九家地方行当','# 第四章')
        for i,(m,b) in enumerate(entries(trades,r'^\* \*\*行当[一二三四五六七八九]+　([^*]+)\*\*'),13):add(f'O{i:02}',m.group(1),b)
        for i,(m,b) in enumerate(entries(chapter(text,'# 第二章','# 第三章'),r'^## 第[^\n]+地域　([^\n]+)'),1):add(f'R{i:02}',m.group(1),b.split('###')[0].strip())
        for i,(m,b) in enumerate(entries(chapter(text,'# 第四章','# 第五章'),r'^## 出身[一二三]　([^\n]+)'),1):add(f'C{i:02}',m.group(1),b.split('## 自定义')[0].strip())
        add('W01','澜川：世界与时代',chapter(text,'# 第一章','# 第二章'))
    return records

source_paths={'lanchuan':'候选剧本/澜川群侠传/澜川群侠传_剧本总案.md','xinghai':'候选剧本/星海争锋/超神机械师同人世界稿.md'}
source_hashes=[]; all_records={}; corrections=[]
for prefix,rel in source_paths.items():
    raw=(args.source_root/rel).read_bytes();text=raw.decode('utf-8-sig')
    source_hashes.append(dict(path=rel,sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw)))
    if prefix=='xinghai':
        # Resolve names against declared people, including references beyond the
        # five originally reported errors. Do not rewrite an ambiguous name.
        people={m.group(1):m.group(2) for m in re.finditer(r'^#### (P\d{2}) ([^｜\n]+)',text,re.M)}
        aliases={}
        for code,name in people.items():
            for alias in re.split(r'[（）]',name):
                if alias and not re.search(r'期|岁|级',alias):aliases[alias]=code
        for alias,code in sorted(aliases.items(),key=lambda x:-len(x[0])):
            pattern=re.compile(re.escape(alias)+r'（(P\d{2})）')
            def fix(m):
                if m.group(1)!=code:corrections.append(dict(name=alias,before=m.group(1),after=code));return alias+'（'+code+'）'
                return m.group()
            text=pattern.sub(fix,text)
        # The ration is a distinct low-grade instance, not the indexed I28.
        text=text.replace('三支临期代用营养膏（I28）','三支临期代用营养膏')
        replacements={
            'S35账目与信用判读':'S36三大文明官僚周旋所学的公文核验',
            'I08速射炮':'I04轻便式动能弹链冲锋炮',
            '发烟筒（I32）':'发烟筒',
            '电磁脉冲手雷（I27）':'电磁脉冲手雷',
            '两块高纯度能量晶体（I36）':'两块衰变晶核碎块（I36）',
            '基因原体样本（I57）':'异化原体诱导代码黑匣子（I57）',
            '《基础能量力学》（S05）':'《进阶能量力学应用》（S05）',
            '《微型动力装甲改装》（I41）':'《微型外骨骼动力核心装配图》（I41）',
            '《磁能武器研发》（S08）':'《重型单兵磁轨狙击枪制造》（S08）',
            '图纸库（I41）':'外骨骼动力核心装配图（I41）',
            '走私暗格（I53）':'以暗格钥匙（I53）开启的走私暗格',
            '反重力阻尼器（S03）':'反重力阻尼器',
            '半管凝胶止血膏（I25）':'半管凝胶止血膏',
            '二手二型外骨骼腿部支架（I09）':'二手二型外骨骼腿部支架',
        }
        for before,after in replacements.items():
            if before in text:corrections.append(dict(before=before,after=after));text=text.replace(before,after)
    all_records[prefix]=extract(prefix,text)
    for record in all_records[prefix]:
        if not record['content'].strip() and record['source_id'].startswith('R'):
            n=int(record['source_id'][1:]);locations=[r['name'] for r in all_records[prefix] if r['source_id'].startswith('L') and (n-1)*5<int(r['source_id'][1:])<=n*5]
            record['content']='这里的主要去处有'+ '、'.join(locations)+'。往来的工作、消息与人际随各处的局势变化。'
canonical_paths={row['path'] for row in source_hashes}
for source in sorted((args.source_root/'候选剧本').rglob('*.md')):
    rel=source.relative_to(args.source_root).as_posix()
    if rel not in canonical_paths:
        raw=source.read_bytes();source_hashes.append(dict(path=rel,sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw),role='retained-comparison-source'))
write(OUT/'source-hashes.json',source_hashes)
write(OUT/'reference-corrections.json',corrections)
write(OUT/'source-records.json',all_records)
for prefix,records in all_records.items():
    source_map=[]
    for r in records:
        row={k:r[k] for k in ('id','source_id','name','region')}
        code=r['source_id'];n=int(code[1:])
        row['source_file']=source_paths[prefix]
        if prefix=='lanchuan':
            chapters={'L':'第二章','O':'第三章','P':'第五章','A':'第六章','E':'第七章','Q':'第八章','S':'第九章','I':'第十章','D':'第十一章','C':'第四章','R':'第二章','W':'第一章'}
            row['original_reference']=chapters[code[0]]+' '+('B'+str(n) if code[0]=='E' else 'P'+str(n) if code[0]=='Q' else code if code[0] in 'AI' else str(n))
        else:row['original_reference']=code
        source_map.append(row)
    write(OUT/f'{prefix}-source-map.json',source_map)
    print(prefix,len(records),{c:sum(r['source_id'].startswith(c) for r in records) for c in 'RLOPAEQSIDCW'})

# These are explicit author balance decisions, not NLP-generated mechanics.
# The source body remains available beside each bounded implementation.
SKILL_KINDS={
    'lanchuan':['heal','guard','guard','heal','guard','heal','control','heal','attack','guard','attack','control','poison','guard','control','attack','attack','guard','control','attack','attack','guard','attack','attack','attack','pilot','pilot','pilot','pilot','social','heal','social','pilot','pilot','social','repair'],
    'xinghai':['engineering','repair','repair','engineering','engineering','engineering','engineering','engineering','repair','engineering','engineering','engineering','attack','heal','attack','guard','attack','guard','control','social','control','heal','control','engineering','social','pilot','control','engineering','social','social','social','pilot','social','pilot','heal','social'],
}
# Only explicit locations or a local institutional workplace are assigned.
# Remote leaders are not placed in an unrelated local site just because the
# manuscript has forty indexed locations. Their contact gate stays remote.
STAR_PLACES={1:3,2:19,5:29,6:26,7:28,9:30,10:26,11:26,13:37,16:31,17:33,18:2,19:11,20:19,21:39,22:38,23:31,24:6,25:1,26:4,27:25,28:1,33:26,41:26,49:1,50:30,51:4,52:3,54:2,55:38,56:6,57:13,58:24,59:39,60:39}
STAR_REMOTE={3:'战争领域',4:'堕落者方舟',8:'虚空维度',12:'狂兽之国',14:'海拉的旅途',15:'低级武道矿星',29:'秘法之殿',30:'星瞳神族领地',31:'光辉联邦超能学院',32:'莫妮莎的机械舰队',34:'拉基的远方旅途',35:'罗泰尔的远方旅途',36:'乔蒂娜的远方旅途',37:'银影的远方旅途',38:'光辉联邦总统近卫驻地',39:'猎日者的重炮操作台',40:'墨瑟的征途',42:'第三圣所的远方遗迹',43:'亥伯龙的远方旅途',44:'圣约的远方据点',45:'白格尔的古老飞船',46:'圣约的隐秘据点',47:'冥土维度',48:'旅者的远方旅途',53:'军刀突击联队营地'}

def make_rules(prefix,records):
    family='wuxia' if prefix=='lanchuan' else 'space'
    rules=dict(family=family,currency='铜钱' if family=='wuxia' else '伊纳',skills=[],items=[],recipes=[],opponents=[],starting_skills=[],starting_items={},starting_currency=0)
    effect_map=[];training_sources=[]
    for r in records:
        code=r['source_id'];n=int(code[1:])
        if code.startswith('S'):
            kind=SKILL_KINDS[prefix][n-1]
            if family=='wuxia':
                rank=7 if n in (1,2,3,4,7,9,10,13,16,18,19,21) else 4 if n<30 else 1
            else:rank=7 if n in (10,12,24) else 4 if n in (6,7,8,9,11,15,17,21,23,29,34) else 1
            scale='ship' if family=='space' and n in (9,28,32,34) else 'machine' if family=='space' and n in (1,2,3,4,5,6,7,8,10,11,12,24,27) else 'person'
            power=min(80,12+rank*7);cost=0 if kind=='social' else 5+rank
            skill=dict(id=r['id'],name=r['name'],kind=kind,scale=scale,power=power,cost=cost,required_rank=rank,training_hours=4 if rank==1 else 24*rank)
            if family=='wuxia' and n==1:skill['effects']=[dict(kind='heal',power=40),dict(kind='energy',power=35),dict(kind='antidote',power=40)]
            if family=='wuxia' and n in (2,6,8):skill['effects']=[dict(kind='guard',power=25),dict(kind='energy',power=30)]
            if family=='wuxia' and n==3:skill['effects']=[dict(kind='guard',power=35),dict(kind='control',power=20)]
            if family=='wuxia' and n==13:skill['effects']=[dict(kind='attack',power=25),dict(kind='poison',power=20)]
            if family=='wuxia' and n==9:skill['conditional_effects']=[dict(required_item='lanchuan-complete-palms',effects=[dict(kind='attack',power=15),dict(kind='recoil',power=2)])]
            if family=='space' and n==22:skill['kind']='energy';skill['power']=30
            if family=='space' and n==22:skill['materials']={'xinghai-i36':1}
            if family=='space' and n==27:skill['materials']={'xinghai-emp':1}
            if family=='space' and n==35:skill.update(materials={'xinghai-nanomed-dose':1},required_tool='xinghai-med-injector')
            if family=='wuxia' and n==31:skill['required_tool']='lanchuan-acupuncture-needles'
            skill['required_item']=prefix+'-training-'+code.lower()
            source=field(r['content'],'学习来源','来源与传授')
            regions=[]
            for loc in records:
                if loc['source_id'].startswith('L') and (loc['name'] in source or loc['source_id'] in source):regions.append(loc['region'])
            for char in records:
                if char['source_id'].startswith('P') and char['name'] in source:
                    cn=int(char['source_id'][1:]);regions.append(char['region'] if family=='wuxia' else next((x['region'] for x in records if cn in STAR_PLACES and x['source_id']==f'L{STAR_PLACES[cn]:02}'),'远方星域'))
            region_names=[x['name'] for x in records if x['source_id'].startswith('R')]
            if not regions:regions=[region_names[(n-1)%len(region_names)]]
            training_sources.append(dict(skill_id=r['id'],item_id=skill['required_item'],name=r['name'],source=source,regions=list(dict.fromkeys(regions)),required_rank=rank,prerequisite=skill.get('prerequisite','')))
            rules['skills'].append(skill)
            effect_map.append(dict(id=r['id'],source_effect=field(r['content'],'效果意图'),binding=dict(kind=kind,scale=scale,power=power),source_limits=field(r['content'],'局限与代价','局限与应对'),status='bounded-adaptation'))
        elif code.startswith('I'):
            kind='tool';effect='';scale='person';price=30;power=0;durability=100
            if family=='wuxia':
                if n<=10:kind='weapon';power=15 if n not in (4,5,10) else 40;price=250 if power==15 else 3000
                elif n<=17:kind='armor';power=8;price=100
                elif n<=26:kind='consumable';effect='poison' if n==26 else 'energy' if n in (22,23) else 'heal';power=25;price=60
                elif n==35:kind='consumable';effect='energy';power=12;price=8
            else:
                if n<=8:kind='weapon';power=20+n*3;price=300+n*100
                elif n in (9,10):kind='machine';scale='machine';power=30;price=1800;durability=150
                elif n<=16:kind='armor';power=12;price=200
                elif 25<=n<=30:kind='consumable';effect='energy' if n in (26,28,29) else 'heal';power=25;price=80
                elif 31<=n<=40:kind='material';price=50
                elif n==58:kind='tool';effect='revive';power=25;price=0
                elif n==59:kind='tool';effect='compute';scale='ship';power=0;price=0
                elif n==60:kind='material';price=0
            item=dict(id=r['id'],name=r['name'],kind=kind,scale=scale,price=price,power=power,durability=durability)
            unique=(family=='wuxia' and (n in (4,6,10) or n>=36)) or (family=='space' and n>=41)
            item['acquisition']='reward' if unique else 'market'
            if unique:item['price']=0
            if '孤品' in r['content'] or (family=='wuxia' and n in (4,6,10,45,50,51,52,53,54,55,56,57,58,59,60)):item['unique']=True
            # I50 denotes a category of individual fallen soldiers' tags. The
            # authored recovery-soldier origin carries three different tags,
            # not three copies of one uniquely owned token.
            if family=='space' and n==50:item.pop('unique',None)
            if effect:item['effect']=effect
            if (family=='wuxia' and n in (20,21)) or (family=='space' and n in (27,30)):item['effect']='antidote'
            if family=='space' and n==58:item['chance']=80
            if family=='space' and n==59:item['multiplier']=4
            if family=='space' and n in (1,2,4,5,6,8):item['materials']={prefix+'-'+{1:'rail-ammo',2:'weapon-cell',4:'belt-ammo',5:'nanite-magazine',6:'pistol-ammo',8:'weapon-cell'}[n]:1}
            if family=='space' and n==7:item.update(kind='consumable',effect='attack')
            rules['items'].append(item)
            effect_map.append(dict(id=r['id'],source_effect=field(r['content'],'实际用途','用途','用途与效果'),binding=dict(kind=kind,scale=scale,effect=effect),status='bounded-adaptation' if effect or power else 'inventory-or-evidence'))
    extras = ([('basic-breath','粗浅吐纳','heal',15),('basic-blade','镖路短刀','attack',15),('basic-fist','船工拳脚','attack',12)] if family=='wuxia' else [])
    for suffix,name,kind,power in extras:rules['skills'].append(dict(id=prefix+'-'+suffix,name=name,kind=kind,scale='person',power=power,cost=5,required_rank=1,training_hours=4,required_item=prefix+'-training-'+suffix))
    extra_items = ([('bread','粗面饼','consumable',3,8,'energy'),('letter','火漆密信','tool',0,0,''),('rusty-blade','豁口短刀','weapon',20,8,''),('dry-flour','炒面干粮','consumable',6,12,'energy'),('wine','烧刀子','consumable',10,8,'energy'),('steel','锻兵生铁','material',30,0,''),('herbs','金疮药草','material',10,0,'')] if family=='wuxia' else [('nutrition','临期代用营养膏','consumable',5,8,'energy'),('half-gel','半管凝胶止血膏','consumable',40,12,'heal'),('leg-brace','二手外骨骼腿部支架','armor',150,5,''),('prybar','合金撬棒','tool',30,0,''),('body-bag','铅皮防辐射收尸袋','tool',30,0,''),('cliff-gloves','防风岩壁手套','tool',30,0,''),('route-map','黑市暗道星图','tool',20,0,''),('mint','劣质醒酒薄荷脑','consumable',8,8,'energy'),('crystal-dust','低纯度能晶碎渣','material',10,0,''),('smoke','发烟筒','consumable',40,10,''),('emp','电磁脉冲手雷','consumable',180,25,''),('frigate','旧式武装轻型护卫舰','ship',12000,40,''),('drone','微型自律跟随浮游炮','machine',1200,25,'')])
    for suffix,name,kind,price,power,effect in extra_items:
        item=dict(id=prefix+'-'+suffix,name=name,kind=kind,scale='ship' if kind=='ship' else 'machine' if kind=='machine' else 'person',price=price,power=power,durability=150 if kind in ('ship','machine') else 100)
        item['acquisition']='craft' if suffix in ('frigate','drone') else 'reward' if suffix in ('letter','body-bag','route-map') else 'market'
        if item['acquisition']=='reward':item['price']=0
        if effect:item['effect']=effect
        if suffix in ('smoke','emp'):item['kind']='material'
        rules['items'].append(item)
    for suffix,name,kind,price in ([('acupuncture-needles','针灸银针','tool',60)] if family=='wuxia' else [('nanomed-dose','纳米促愈注射剂','material',80),('med-injector','医用高压注射枪','tool',120)]):
        rules['items'].append(dict(id=prefix+'-'+suffix,name=name,kind=kind,scale='person',price=price,power=0,durability=100,acquisition='market'))
    if family=='space':
        for suffix,name,price in [('rail-ammo','磁轨枪弹丸',5),('belt-ammo','弹链机炮弹药',4),('pistol-ammo','动能手枪弹',2),('weapon-cell','武器电池',6),('nanite-magazine','纳米蜂群弹匣',12)]:
            rules['items'].append(dict(id=prefix+'-'+suffix,name=name,kind='material',scale='person',price=price,power=0,durability=1,acquisition='market'))
    def recipe(suffix,name,skill,materials,output,hours=4):
        rules['recipes'].append(dict(id=prefix+'-'+suffix,name=name,skill_id=prefix+'-'+skill,skill_rank=1,hours=hours,materials={prefix+'-'+k:v for k,v in materials.items()},output_id=prefix+'-'+output,output_count=1))
    if family=='wuxia':
        recipe('forge-blade','锻制雁翎刀','s36',{'steel':3},'i01',8)
        recipe('prepare-salve','配制三黄金疮药','s32',{'herbs':3},'i18',4)
        recipe('forge-sword','锻制青钢剑','s36',{'steel':4},'i02',12)
        start_skills=['basic-fist','s34'];start_items={'rusty-blade':1,'bread':3,'letter':1};coins=0
    else:
        recipe('assemble-exoskeleton','装配二型猎兵外骨骼','s03',{'i33':4,'i31':2,'i37':2},'i09',12)
        recipe('assemble-drone','装配跟随浮游炮','s07',{'i33':3,'i34':2,'i31':1},'drone',12)
        recipe('assemble-rifle','制造磁轨狙击枪','s08',{'i33':2,'i34':3,'i38':2},'i01',12)
        recipe('assemble-frigate','重建武装轻型护卫舰','s10',{'i33':30,'i34':20,'i31':10,'i39':10},'frigate',120)
        recipe('boost-shield','改装折叠动能护盾','s06',{'i33':2,'i38':2,'i31':1},'i11',8)
        start_skills=['s01','s02'];start_items={'i17':1,'half-gel':1,'i49':1,'nutrition':3};coins=0
    rules['starting_skills']=[prefix+'-'+s for s in start_skills];rules['starting_items']={prefix+'-'+i:n for i,n in start_items.items()};rules['starting_currency']=coins
    for source in training_sources:
        rules['items'].append(dict(id=source['item_id'],name=source['name']+'传授凭证',kind='material',scale='person',price=0,power=0,durability=1,acquisition='reward'))
    for suffix,name,kind,power in extras:rules['items'].append(dict(id=prefix+'-training-'+suffix,name=name+'习练手记',kind='material',scale='person',price=0,power=0,durability=1,acquisition='reward'))
    for skill in rules['starting_skills']:
        suffix=skill.removeprefix(prefix+'-');rules['starting_items'][prefix+'-training-'+suffix]=1
    # Extra goods are authored by the quest owner and merged by stable identity.
    goods_path=OUT/f'{prefix}-quest-goods.json'
    if goods_path.exists():
        existing={i['id'] for i in rules['items']}
        for good in json.loads(goods_path.read_text(encoding='utf-8')):
            if good['id'] not in existing:
                item=dict(id=good['id'],name=good['name'],kind=good.get('kind','material'),scale='person',price=0,power=0,durability=1,acquisition='reward')
                if good['id'] in ('lanchuan-mine-pigment','lanchuan-lute-strings','xinghai-antifungal'):item.update(acquisition='market',price=200)
                if good['id']=='xinghai-light-mark':item.update(kind='tool',effect='revive',chance=100,unique=True)
                rules['items'].append(item);existing.add(good['id'])
    rules['repair_material_id']=prefix+('-steel' if family=='wuxia' else '-i33')
    market_locations=[1,6,11,17,21,28,32,36] if family=='wuxia' else [2,8,11,17,22,24,27,30]
    rules['markets']=[]
    for n in market_locations:
        loc=next(x for x in records if x['source_id']==f'L{n:02}')
        rules['markets'].append(dict(id=prefix+f'-market-l{n:02}',name=loc['name']+'市集',location=loc['name'],location_aliases=[loc['id']],items=[i['id'] for i in rules['items'] if i.get('acquisition')=='market']))
    for r in records:
        if not r['source_id'].startswith('E'):continue
        n=int(r['source_id'][1:]);scale='person' if family=='wuxia' else ['person','person','person','person','machine','person'][n-1]
        opponent=dict(id=r['id'],name=r['name'],scale=scale,health=180+n*40,attack=20+n*5,defense=8+n*3,reward_currency=100+n*50,reward_experience=30+n*10)
        opponent.update(rank=([5,7,7,6,8,9] if family=='wuxia' else [4,5,8,6,7,9])[n-1],energy=100,lethal=(n in (3,4) if family=='wuxia' else True))
        person=({1:7,2:36,3:47,4:50,5:28,6:14} if family=='wuxia' else {2:23,3:17}).get(n)
        if person:opponent['character_source_key']=prefix+f'-p{person:02}'
        location_number=([4,23,30,33,19,10] if family=='wuxia' else [7,31,33,17,38,39])[n-1]
        loc=next(x for x in records if x['source_id']==f'L{location_number:02}')
        opponent.update(location=loc['name'],location_aliases=[loc['id']])
        if family=='wuxia':opponent['skill_ids']=[prefix+'-'+{1:'s23',2:'s18',3:'s13',4:'s25',5:'s12',6:'s16'}[n]]
        rules['opponents'].append(opponent)
    loc=next(x for x in records if x['source_id']=='L02' if family=='wuxia') if family=='wuxia' else next(x for x in records if x['source_id']=='L03')
    rules['opponents'].append(dict(id=prefix+'-sparring',name='切磋对手' if family=='wuxia' else '训练靶机',location=loc['name'],location_aliases=[loc['id']],scale='person',health=60,attack=8,defense=3,reward_currency=0,reward_experience=10))
    if family=='space':
        loc=next(x for x in records if x['source_id']=='L22')
        rules['opponents'].append(dict(id='xinghai-raider-frigate',name='碎石带掠夺船',location=loc['name'],location_aliases=[loc['id']],scale='ship',health=360,attack=40,defense=25,reward_currency=450,reward_experience=80))
    write(OUT/f'{prefix}-effect-map.json',effect_map)
    write(OUT/f'{prefix}-training-sources.json',training_sources)
    return rules

def make_pack(prefix,records):
    is_wuxia=prefix=='lanchuan';family='wuxia' if is_wuxia else 'space'
    world_name='澜川群侠传' if is_wuxia else '星海争锋：星海历689'
    rules=make_rules(prefix,records);idx={r['source_id']:r for r in records}
    rules['npcs'],npc_evidence=build_npc_catalog(prefix,records,rules)
    write(OUT/f'{prefix}-npc-source-mapping.json',npc_evidence)
    desc='深秋的澜川，船户、镖师、医者与十二门派各谋生计。你从渡口、镖局或山下茶行出发，凭手艺与选择走出自己的江湖。' if is_wuxia else '星海历689年，朱伯利船坞仍在开工，苏尼尔前线急需物资，龙坦的商贩照常议价。你可以修船谋生、护送货物、调查旧事，逐渐争取自己的位置。'
    pack=dict(version=1,id='lanchuan-qunxia' if is_wuxia else 'xinghai-689',name=world_name,description=desc,tags=['武侠','江湖','平行世界'] if is_wuxia else ['星际','机械','同人'],content_version=1,recommended_planning_template_id='episodic-emergent',world=[],characters=[],protagonist_templates=[])
    types={'L':'location','R':'location','O':'organization','P':'character','I':'item','S':'setting','A':'event','E':'character','Q':'event','D':'setting','W':'setting','C':'setting'}
    for r in records:
        code=r['source_id'];content=r['content'];brief=''
        if code.startswith('P'):brief=field(content,'初见简介','身份动向')
        elif code.startswith('L'):brief=field(content,'初到景象','第一印象','初见景象')
        elif code.startswith('A'):brief=field(content,'最初消息','初始线索')
        if not brief:brief=r['name']+'。'
        entry=dict(id=r['id'],name=r['name'],type=types[code[0]],importance='high' if code=='W01' else 'medium',tags=[r['region']] if r['region'] else [],keywords=[r['name'],code],load_mode='resident' if code=='W01' else 'auto',brief_description=brief,content=content)
        # The lore store has unique display names within a world. Encounters
        # reference the existing person; they must not create a second person.
        if code.startswith('E'):entry.update(name=r['name']+'：交锋与抉择',type='event')
        if r['id']=='lanchuan-o18':entry['name']='赤炉庄铸剑行当'
        if r['id']=='xinghai-o04':entry['name']='机械国度势力'
        if code.startswith('P'):
            n=int(code[1:]);name=r['name'];role=field(content,'所在地／身份','身份动向')
            place=role.split('·')[0] if is_wuxia else idx[f'L{STAR_PLACES[n]:02}']['name'] if n in STAR_PLACES else STAR_REMOTE[n]
            location=next((x for x in records if x['source_id'].startswith('L') and (x['name'] in place or place in x['name'])),None)
            region=r['region'] if is_wuxia else idx[f'L{STAR_PLACES[n]:02}']['region'] if n in STAR_PLACES else '远方星域'
            gender='女' if re.search(r'[（｜]女[，｜]',r['title']) else '男' if re.search(r'[（｜]男[，｜]',r['title']) else ''
            age_match=re.search(r'(?<!外貌)(?:[，（]约?)(\d+)岁',r['title'])
            age=int(age_match.group(1)) if age_match and '外貌' not in r['title'] else 0
            if age:entry['content']='实际年龄：'+str(age)+'岁。\n'+content
            aliases=[]
            if not is_wuxia:
                alias=re.search(r'^[^（]+（([^）]+)）',r['title'])
                if alias and not re.search(r'期|岁|级|/|少女',alias.group(1)):aliases=[alias.group(1)]
            gate='须先获得引荐或本人同意，约定地点后方可会面。' if (not is_wuxia and (n<=15 or 29<=n<=48)) or (is_wuxia and n in (14,25,28,33,34,40,48,54,55,56)) else '可在其日常活动地点接触，是否交谈由当时处境决定。'
            pack['characters'].append(dict(source_key=r['id'],name=name,aliases=aliases,country=region,role=role,topics=[region] if region else [],initial_place_candidate=location['id'] if location else '',initial_place_label=place,region_tokens=[place] if place else [],level='重要' if '须先' in gate else '普通',brief=brief,lore_id=r['id'],contact_gate=gate,age=age,gender=gender,speech_style=field(content,'说话风骨','说话声口')))
        pack['world'].append(entry)
    # Independent secret dossiers are never resident or public character briefs.
    if not is_wuxia:
        pack['world'].append(dict(id='xinghai-w03',name='平行星海的时代说明',type='setting',importance='high',tags=['时代'],keywords=['星海历689','平行'],load_mode='resident',brief_description='星海历689年的平行同人世界。',content='这是以星海历689年为开局的平行同人世界。人物目录中的未来称号和远景成就是作者展望，不是开局已发生的事实。黑星佣兵团仍处于初创阶段；远方巨头各有自己的事务，玩家必须通过行程、引荐或正式通信建立接触。玩家不继承黑星的身份或未来。'))
    origin_order=[3,2,1] if is_wuxia else [1,2,3]
    origins=[]
    for n in origin_order:
        r=idx[f'C{n:02}'];profile=field(r['content'],'选角简介','背景概述') or r['content'].splitlines()[0]
        loc=field(r['content'],'起步地点','起点位置')
        items=dict(rules['starting_items']);skills={s:1 for s in rules['starting_skills']};currency=0
        if is_wuxia and n==1:items={prefix+'-'+s:1 for s in ('i48','i15','i37','i02')};skills={prefix+'-basic-breath':1};currency=30
        if is_wuxia and n==2:items={prefix+'-'+s:1 for s in ('i01','i13','i34','i29','dry-flour','wine')};skills={prefix+'-basic-blade':1}
        if not is_wuxia and n==2:items={prefix+'-leg-brace':1,prefix+'-prybar':1,prefix+'-body-bag':1,prefix+'-i50':3};skills={prefix+'-s13':1,prefix+'-s35':1}
        if not is_wuxia and n==3:items={prefix+'-'+s:1 for s in ('cliff-gloves','route-map','mint','crystal-dust')};skills={prefix+'-s30':1}
        for skill in skills:items[prefix+'-training-'+skill.removeprefix(prefix+'-')]=1
        st=dict(family=family,rank=1,experience=0,health=100,max_health=100,energy=100,max_energy=100,injury=0,currency=currency,skills=skills,training={},inventory=items,equipment={},assets={})
        if not is_wuxia and n==1:
            st['opening_hazards']=[dict(id='xinghai-c01-vacuum',due_seconds=6000,location_ids=['xinghai-l01'],escape_location_id='xinghai-l02',damage=40,injury=20,description='47号长廊将在一小时四十分后开始真空吹扫；先离开危险区。欠费不会因离开自动消失。',consequence='吹扫按时开始，你在强制疏散中受伤，被送往老锚酒吧一侧的安全气闸。')]
        # Keep the choice before acceptance: original scripted openings sometimes
        # made the protagonist take a job or speak without player input.
        opening=('潮水刚退，渡船上的旧缆绳仍湿着。宋老橹坐在茅屋外补网，留出半张凳子。蓝阿鲤从村道走来，提着一篮没卖完的鱼，问今日是否有人往澜口城去。怀中的火漆信还封着，去两淮的路很长，今天先怎么走，由你决定。' if is_wuxia and n==3 else '雁行镖局的后院响着练刀声。晏小五递来一碗热饭，前厅的铁云板随后敲了三下。霍千里把一张去清河城的签条放在桌上，询问谁愿意跟车；你还可以先问清报酬与路况。' if is_wuxia and n==2 else '迎仙镇的茶行刚卸下门板，涂大壶把热汤放在你面前。山上的云还没散，孤松亭的石阶湿得发亮。重阳前还有些日子，掌柜说，想往哪边走，先吃口热的。' if is_wuxia else '朱伯利外环的灯管亮了一半。蒙托的账单压在工具箱上，旧管道仍需要人修。老锚托伙计带来话：船坞有一单临时组装活，可以先谈工钱，也可以去港务处核对欠费。远处传来试机的低鸣，你还没有答应任何人。' if n==1 else '铁铸要塞的后勤营房里，莉娜正在清点绷带。警报从三号闸门方向传来，雷尔蒙特让辅兵先确认撤离通道。你手边是撬棒、旧支架和三枚待交还的铭牌；营房后门通往医疗站，前面则是拥挤的整备坑道。' if n==2 else '龙坦岩壁上的商摊刚刚开灯。送货的账房在等回话，石阶下却有人跌倒，掌心攥着一枚染血的芯片。两名披斗篷的人沿坡走来。你可以去找巡卫、帮助伤者，也可以先躲进熟悉的岔道。')
        energy_name='内力' if is_wuxia else '气力'
        if not is_wuxia and n==1:opening='朱伯利外环的灯管亮了一半。蒙托的催缴无人机堵在集装箱门口，红光照着那张欠费账单。长廊广播响起：距例行真空吹扫还有一小时四十分。老锚酒吧一侧的安全气闸仍开着，船坞也有人招临时组装工；先往哪里去，必须尽快拿主意。'
        template=dict(id=r['id'],name=r['name'],description=profile,character_name='行者' if is_wuxia else '旅人',profile=profile,origin=loc+'\n'+profile,opening_text=opening,state={'姓名':'行者' if is_wuxia else '旅人','基本身份':profile,'当前处境':loc,'历练状态':st,'历练等级':1,'生命':100,'生命上限':100,energy_name:100,energy_name+'上限':100,'伤势程度':0,'钱财':currency})
        pack['protagonist_templates'].append(template)
        origins.append(dict(id=r['id'],name=r['name'],description=profile,template_id=r['id'],default=n==origin_order[0]))
    first=pack['protagonist_templates'][0]
    pack['story']=dict(title=world_name,origin=first['origin'],protagonist_name=first['character_name'],protagonist=first['profile'],opening_text=first['opening_text'],reply_target_chars=2500,choice_count=3,module_refs={'event_package_ids':['default'],'actor_state_id':family+'-state-v1','rule_system_id':family+'-rules-v1','image_preset_id':'realistic'},gameplay=dict(status_template=family+'-state-v1',numeric_scale=family,growth=family,combat=family,communication='letter' if is_wuxia else 'radio',letter_delivery_days=1,opening='scene',confirmed=True,adventure_rules=rules),narrative_requirements='先写眼前的生活与人物行动，让玩家决定是否承接事务。公开介绍与作者秘密分别使用；秘密只在取得证据后揭示。人物按所在地域、日常与接触门槛登场。原稿能力是传承说明，当前结果以已登记规则和实际结算为准，不凭正文凭空发钱、发物或授予能力。')
    pack['creation']=dict(age_range=[20,28] if is_wuxia else [20,26],genders=[dict(id='male',name='男',default=True),dict(id='female',name='女')],origins=origins,state_basis_templates=[r['id'] for r in pack['protagonist_templates']],affiliations=[dict(id='none',name='随出身',description='沿用所选出身已有的归属与人际。',kind='none',default=True)],locations=[dict(id='auto',name='随出身起点',description='从所选出身的地点开始。',default=True)],effect_notes={'origins':'narrative','affiliations':'narrative','locations':'narrative'})
    return pack

def adapt_effect_text(prefix,record,definition):
    """Reviewed player prose follows only effects that the reducer consumes.

    Original prose and the external model's candidate remain in evidence. This
    deterministic editorial merge prevents an attractive paraphrase from
    resurrecting an unsupported promise or converting hours into double-hours.
    """
    is_skill=record['source_id'].startswith('S');body=record['content'];wuxia=prefix=='lanchuan'
    energy='内力' if wuxia else '气力'
    origin=field(body,'传承来历') if is_skill else '\n'.join(x for x in [field(body,'玩家可见描述'),field(body,'来历与背景','来历背景','来历','外观与来历','外观')] if x)
    if not origin:origin=record['name']+'是'+('澜川江湖' if wuxia else '星海旅途')+'中流传的'+('一门本领。' if is_skill else '一件物品。')
    descriptions={
        'attack':'施展开来，可以对眼前的对手造成伤害。',
        'heal':'行功或施救时，可以恢复一些生命；重伤仍需另外休养。',
        'energy':'运用得当，可以恢复一些'+energy+'，继续应付眼前的交锋。',
        'guard':'施展开来，可以稳住这一轮的守势，减轻随后的来袭。',
        'control':'看准时机出手，可以打断对手，让其在这一轮无法反击。',
        'poison':'毒性可以在这场交锋中持续损伤对手，直至得到相应处理。',
        'antidote':'可以清除眼下交锋中积下的毒性。',
        'repair':'凭此手艺，备好材料并花时间检修，可以恢复受损器械的耐久。',
        'engineering':'掌握这门手艺后，可以参与相应的制作与工程委托；具体所需材料和工时须事先备齐。',
        'pilot':'运用这门本领，可以在一轮交锋中稳住防守，也可应对相关委托中的操纵考验。',
        'social':'掌握这门学问后，可以在相关委托中辨别情况、运用相应的应对办法；对方是否答应，还要看事情本身。',
    }
    if is_skill:
        effects=definition.get('effects') or [dict(kind=definition['kind'],power=definition['power'])]
        effect=''.join(descriptions[e['kind']] for e in effects)
        if record['id']=='lanchuan-s36':effect='熟悉这门手艺后，可以按学会的锻造方法准备生铁与工时，打制可实际携带和使用的兵刃。'
        if definition['scale']=='ship':effect='这门本领用于舰船上的操作。'+effect
        elif definition['scale']=='machine':effect='这门本领用于操作或维护机械。'+effect
        source=field(body,'学习来源','来源与传授')
        learn='先获得传授或研习手札，再花时间练习，才算真正学会。'
        if source:learn+='传授线索在'+source.rstrip('。')+'。'
        content=origin+'\n\n'+effect+'\n\n'+learn
        if record['id']=='xinghai-s22':content+='每次抽取都要消耗一份衰变晶核碎块。'
        if record['id']=='xinghai-s27':content+='每次布雷都要消耗一枚电磁脉冲手雷。'
        if record['id']=='xinghai-s35':content+='施救时须有医用高压注射枪，每次消耗一份纳米促愈注射剂。'
        if record['id']=='lanchuan-s31':content+='施针时须随身备有针灸银针。'
        if record['id']=='lanchuan-s09':content+='残掌旧案若能走到重铸传承的一步，完整掌意还会带来更重的攻势，同时让自身承受反震。'
        brief=effect
    else:
        kind=definition['kind'];e=definition.get('effect','')
        effect={'weapon':'握持并装备好后，可以增强同一类交锋中的攻击。','armor':'穿戴妥当后，可以帮助抵挡同一类交锋中的来袭。','tool':'它可作为相关事务中交付、查验或使用的实物；带在身上并不等于已经办成其中的事情。','material':'它可以作为相应制作或委托所需的实物材料；使用和交付都会核对手中实有的数量。','machine':'整备后可以投入机械交锋；运转需要燃料，受损后要备料维修。','ship':'整备后可以投入舰船交锋；护盾、船体和燃料都要留意，远航前应备好维修物资。','consumable':'使用后会消耗一份。'}[kind]
        if e=='heal':effect='使用一份可以恢复一些生命；药物或补给耗尽后，需要另找来源。'
        elif e=='attack':effect='在个人交锋中投掷一枚，可以伤及眼前的对手；引爆后就不能再次使用。'
        elif e=='energy':effect='使用一份可以恢复一些'+energy+'，物品随之消耗。'
        elif e=='poison':effect='在交锋中使用，可以使对手受到持续的毒性损伤；每次会消耗一份。'
        elif e=='antidote':effect='使用一份可以清除当前交锋中的中毒，不会留下永久的免毒本领。'
        elif e=='revive':effect='佩戴在身上后，遭遇一次个人交锋中的致命打击时，有'+str(definition['chance'])+'%的机会留住一线生机并脱离交锋。无论是否奏效，它都会耗尽；持续的高温或真空另有其危险。'
        elif e=='compute':effect='装入舰船工具位后，可以将舰船技能的效力提升至原来的四倍。它只为这艘船的相应操作提供支援。'
        if record['id']=='xinghai-i29':effect='这是供短时应急使用的小罐补给。用完一份可以恢复一些气力，罐体随即耗尽；它并不能保证在真空中维持一段安全时间。'
        source=field(body,'获取出处','获得地点与方式','来源','获得来源','获得方式')
        acquisition='可向出售它的当地商家询价。' if definition.get('acquisition')=='market' else '须在相关事件中真正取得。' if definition.get('acquisition')=='reward' else '需要备齐材料并按学会的方法制作。'
        content=origin+'\n\n'+effect+'\n\n'+('寻物线索：'+source.rstrip('。')+'。' if source else '')+acquisition
        if definition.get('materials'):content+='出手时要消耗对应的弹药或武器供能物资，缺了补给便不能照常使用。'
        if record['id']=='lanchuan-i10':content=origin+'\n\n这柄剑仍由容九皋持有。你可以追寻剑与烟波庄的往事，剑主的认可并不等于将他的佩剑带走。'
        if record['id']=='lanchuan-i50':content=origin+'\n\n这是丐帮帮主掌管的重器。帮中信任与武学传授可以另行争取，棒本身仍归帮主管领。'
        brief=effect
    return content,brief

for prefix,records in all_records.items():
    pack=make_pack(prefix,records)
    definitions={x['id']:x for x in pack['story']['gameplay']['adventure_rules']['skills']+pack['story']['gameplay']['adventure_rules']['items']}
    source_index={x['id']:x for x in records}
    player_records=[]
    for entry in pack['world']:
        if entry['id'] in definitions and source_index[entry['id']]['source_id'][0] in 'SI':
            entry['content'],entry['brief_description']=adapt_effect_text(prefix,source_index[entry['id']],definitions[entry['id']])
            player_records.append(dict(id=entry['id'],name=entry['name'],content=entry['content']))
    write(OUT/f'{prefix}-player-effects.json',player_records)
    write(ROOT/'internal/storypack/assets'/f'{pack["id"]}.json',pack)
    write(OUT/f'{prefix}-adventure-rules.json',pack['story']['gameplay']['adventure_rules'])
