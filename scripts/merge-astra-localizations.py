"""Merge reviewed display-only English fields by stable source identity."""
import argparse,json,re
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--evidence',type=Path,required=True);a=p.parse_args()
root=Path(__file__).resolve().parents[1];assets=root/'internal/storypack/assets'
def write(path,data):path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
english={};review=[]
for pattern in ('english-candidate-*.json','player-english-candidate-*.json'):
    for f in sorted(a.evidence.glob(pattern)):
        data=json.loads(f.read_text(encoding='utf-8'))
        if not data.get('ok'):raise ValueError('Unreviewed translation batch: '+f.name)
        for item in data['items']:
            if not item['name'].strip() or not item['content'].strip():raise ValueError('Empty translation '+item['id'])
            english[item['id']]=item
        review.append(dict(file=f.name,items=len(data['items']),model=data.get('model')))

EXTRA={
'basic-breath':'Basic Breathing Practice','basic-blade':'Caravan Short-Blade Work','basic-fist':'Boatworker Fistwork','bread':'Coarse Flatbread','letter':'Wax-Sealed Letter','rusty-blade':'Chipped Short Blade','dry-flour':'Toasted Flour Rations','wine':'Strong Grain Liquor','steel':'Forging Iron','herbs':'Wound-Salve Herbs',
'nutrition':'Near-Expiry Nutrient Paste','half-gel':'Half a Tube of Wound Gel','leg-brace':'Second-Hand Exoskeleton Leg Brace','prybar':'Alloy Prybar','body-bag':'Lead-Lined Recovery Bag','cliff-gloves':'Cliffside Work Gloves','route-map':'Black-Market Passage Map','mint':'Cheap Mint Sobering Tonic','crystal-dust':'Low-Purity Energy Crystal Shards','smoke':'Smoke Canister','emp':'EMP Grenade','frigate':'Used Armed Light Frigate','drone':'Autonomous Escort Drone',
'shore-shoes':'Xiaoliu’s Woven Shore Shoes','dried-fish':'White Egret Village Dried Fish','spring-flour':'First-Harvest Wheat Flour','lake-map':'Shi Bugui’s Qingping Lake Map','lute-strings':'Old Silkworm-Silk Lute Strings','mine-pigment':'Malachite Green Pigment','soft-knee-cloth':'Song Qing’s Embroidered Knee Wrap','old-bowl':'Qi Wujiu’s Old Earthen Bowl','prayer-notes':'Ji Wenmo’s Handwritten Notes','sealed-note':'Rong Huaijin’s Unopened Letter','complete-palms':'Complete Eighteen Palms Transmission',
'antifungal':'Antifungal Medicine','water-medal':'Sunil Greenblood Medal','knight-medal':'Godoran Service Medal','light-mark':'Aroshia’s Protective Glimmer','naval-seal':'Imperial Special Stargate Seal',
'forge-blade':'Forge a Goose-Feather Sabre','prepare-salve':'Prepare Triple-Yellow Wound Salve','forge-sword':'Forge a Steel Sword','assemble-exoskeleton':'Assemble a Type-II Hunter Exoskeleton','assemble-drone':'Assemble an Escort Drone','assemble-rifle':'Build a Rail Sniper Rifle','assemble-frigate':'Rebuild an Armed Light Frigate','raider-frigate':'Asteroid-Belt Raider Frigate',
'acupuncture-needles':'Silver Acupuncture Needles','nanomed-dose':'Nanomedical Healing Dose','med-injector':'Medical Pressure Injector','boost-shield':'Modify a Folding Kinetic Shield',
'rail-ammo':'Railgun Slugs','belt-ammo':'Autocannon Belt Ammunition','pistol-ammo':'Kinetic Pistol Rounds','weapon-cell':'Weapon Power Cell','nanite-magazine':'Nanite-Swarm Magazine',
}

def strip_label(line):
    line=re.sub(r'^\s*[-*]\s*','',line)
    return re.sub(r'^\*\*[^*]+\*\*\s*[:：]?\s*','',line).strip()

for pack_id,prefix in [('lanchuan-qunxia','lanchuan'),('xinghai-689','xinghai')]:
    pack=json.loads((assets/(pack_id+'.json')).read_text(encoding='utf-8'))
    world={};characters={};templates={};adventure={}
    for entry in pack['world']:
        key=entry['id']
        if key=='xinghai-w03':
            e=dict(name='A Parallel Star Ocean',content='This fan-fiction world opens in the year 689 of the Galactic Calendar. Future titles and achievements in the author’s catalog are possibilities, not events already completed at the opening. Black Star’s mercenary group is still being established. Distant leaders have their own business; meeting them requires travel, introductions or actual communication. The protagonist does not inherit Black Star’s identity or future.')
        else:e=english[key]
        content=e['content'].replace('petty江湖 feuds','petty jianghu feuds')
        additions={'lanchuan-s31':'Treatment requires a set of silver acupuncture needles.','xinghai-s22':'Each extraction consumes one decaying energy-core fragment.','xinghai-s27':'Each deployment consumes one EMP grenade.','xinghai-s35':'Treatment requires a medical pressure injector and consumes one nanomedical healing dose.'}
        if key in additions and additions[key] not in content:content+='\n\n'+additions[key]
        if key=='lanchuan-i10':content=content.split('\n\n')[0]+'\n\nRong Jiugao retains this sword. You may investigate its history and that of Mistwater Manor; earning the swordsman’s respect does not transfer ownership of his blade.'
        if key=='lanchuan-i50':content=content.split('\n\n')[0]+'\n\nThis emblematic staff remains in the keeping of the Beggar Sect’s leader. Trust and instruction may be earned separately; the staff itself remains the leader’s charge.'
        if key in ('xinghai-i01','xinghai-i02','xinghai-i04','xinghai-i05','xinghai-i06','xinghai-i08'):content+='\n\nEach attack consumes the corresponding ammunition or weapon power supply; the weapon cannot operate normally without it.'
        if key=='xinghai-i07':content=content.split('\n\n')[0]+'\n\nThrowing one in personal combat damages the opponent in front of you. Detonation consumes the grenade; it cannot be reused or treated as a ship-destroying weapon. Local sellers may offer replacements.'
        world[key]=dict(name=e['name'],brief_description=e['name']+'.',content=content,tags=[])
        if key[-3:-2]=='e':world[key]['name']=e['name']+' — Confrontation and Choices'
        if key in ('lanchuan-o18','xinghai-o04'):world[key]['name']=e['name']+' — Organization'
        if key[-3:-2] in ('s','i'):adventure[key]=e['name']
    for c in pack['characters']:
        e=english[c['source_key']]
        lines=[x.strip() for x in e['content'].splitlines() if x.strip() and not x.startswith('#')]
        role=strip_label(lines[0])
        # Character source cards consistently put public introduction third in
        # Lanchuan and identity first in Xinghai; secrets occur later, separately.
        brief=strip_label(lines[2]) if prefix=='lanchuan' and len(lines)>2 else role
        place=world.get(c.get('initial_place_candidate'),{}).get('name')
        if not place:
            candidates=[w['name'] for w in pack['world'] if w['type']=='location' and (w['name'] in c['initial_place_label'] or c['initial_place_label'] in w['name'])]
            place=next((world[w['id']]['name'] for w in pack['world'] if w['name'] in candidates),'Location described in the character’s profile')
        characters[c['source_key']]=dict(name=e['name'],brief=brief,role=role,initial_place_label=place)
        world[c['lore_id']]['brief_description']=brief
    origin_text={
        'lanchuan-c01':('Half a Daoist Manuscript','A lay disciple of Wudang, starting in Yingxian Town with a practice manuscript, a sword and thirty copper coins.'),
        'lanchuan-c02':('A Bowl of Escort Rice','A working caravan escort in Hedu Town, with issued equipment, fellow escorts and an advance to repay.'),
        'lanchuan-c03':('A Visitor at the Ferry','A former salt-boat worker staying at Tidewatch Ferry, carrying three flatbreads, a chipped blade and a sealed letter.'),
        'xinghai-c01':('The Indebted Maintenance Worker','A repair worker on Juberly’s outer ring, carrying worn tools and looking for work while resolving berth fees.'),
        'xinghai-c02':('The Sunil Frontline Recovery Soldier','A support soldier at the Ironcast Fortress, equipped with a used leg brace and tasked with returning fallen soldiers’ tags.'),
        'xinghai-c03':('The Floating Dragon Errand Runner','A cliff-market apprentice with gloves, a passage map and local contacts, free to choose how to face an unexpected pursuit.'),
    }
    for t in pack['protagonist_templates']:
        name,desc=origin_text[t['id']];templates[t['id']]=dict(name=name,description=desc)
    rules=pack['story']['gameplay']['adventure_rules']
    for group in ('skills','items','recipes','opponents','markets'):
        for r in rules[group]:
            key=r['id'];suffix=key.removeprefix(prefix+'-')
            if key in english:adventure[key]=english[key]['name']
            elif suffix.startswith('training-'):
                skill_id=prefix+'-'+suffix.removeprefix('training-')
                name=english.get(skill_id,{}).get('name',EXTRA.get(suffix.removeprefix('training-')))
                if not name:raise ValueError('Missing training translation '+key)
                adventure[key]=name+' — Teaching Notes'
            elif suffix.startswith('market-'):
                location_id=prefix+'-'+suffix.removeprefix('market-')
                adventure[key]=world[location_id]['name']+' Market'
                r['location_aliases']=list(dict.fromkeys(r.get('location_aliases',[])+[world[location_id]['name']]))
            elif suffix=='sparring':adventure[key]='Sparring Partner' if prefix=='lanchuan' else 'Practice Drone'
            elif suffix in EXTRA:adventure[key]=EXTRA[suffix]
            else:raise ValueError('Missing adventure translation '+key)
            if group=='opponents':
                loc=next((w for w in pack['world'] if w['type']=='location' and w['name']==r['location']),None)
                if loc:r['location_aliases']=list(dict.fromkeys(r.get('location_aliases',[])+[world[loc['id']]['name']]))
    title='Heroes of Lanchuan' if prefix=='lanchuan' else 'Star Ocean Rivalry: Galactic Year 689'
    localization=dict(name=title,title=title,description='Follow the ferries, escort roads and mountain paths of Lanchuan. Make a living, learn a craft, investigate old debts and decide where your loyalties lie.' if prefix=='lanchuan' else 'Work the docks of Juberly, face the Sunil frontier or trade along Floating Dragon’s cliffs. Build your skills and choose which people and causes to stand beside.',tags=['Wuxia','Open World','Parallel History'] if prefix=='lanchuan' else ['Space','Engineering','Fan Fiction'],protagonist='Wanderer' if prefix=='lanchuan' else 'Traveler',world=world,characters=characters,templates=templates,creation=dict(origins={key:dict(name=n,description=d) for key,(n,d) in origin_text.items() if key.startswith(prefix)},genders={'male':dict(name='Male'),'female':dict(name='Female')},affiliations={'none':dict(name='From your background',description='Keep the affiliations and relationships of your chosen background.')},locations={'auto':dict(name='Background starting point',description='Begin at the location belonging to your chosen background.')}),adventure=adventure,initial_state=dict(items={i['name']:adventure[i['id']] for i in rules['items']},techniques={s['name']:adventure[s['id']] for s in rules['skills']}))
    write(assets/(pack_id+'.en-US.json'),localization)
    write(assets/(pack_id+'.json'),pack)
    print(pack_id,len(world),len(characters),len(adventure))
write(a.evidence/'localization-merge-review.json',dict(batches=review,merged_by='stable_id',player_effect_prose='Human-reviewed deterministic adaptation; external effect candidates retained for audit, not blindly applied.'))
