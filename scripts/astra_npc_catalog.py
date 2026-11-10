"""Period-specific NPC baselines and sourced schedules, shared with actor rules.

Ranks and HP are adaptation parameters, not quotations of manuscript statistics.
Skill assignments use only a described vocation, school or explicit technique;
none of the distant characters receives a fictional local meeting location.
"""
import re

# Person number -> (adapted rank, already practised catalog techniques).
WUXIA={
1:(5,[34]),2:(2,[35]),3:(3,[35]),4:(5,[17]),5:(3,[17]),6:(3,[34]),7:(5,[23]),8:(6,[30,35]),9:(7,[23,33]),10:(3,[30,33]),
11:(6,[34]),12:(4,[34]),13:(4,[35]),14:(9,[16]),15:(2,[32,34]),16:(4,[17]),17:(7,[9,19]),18:(5,[35]),19:(6,[19]),20:(2,[35]),
21:(7,[30,35]),22:(2,[35]),23:(3,[35]),24:(4,[31,32]),25:(9,[2,12,15]),26:(7,[12,22]),27:(1,[]),28:(8,[12]),29:(6,[17]),30:(2,[35]),
31:(1,[]),32:(2,[35]),33:(8,[6,10,18]),34:(8,[6,10,18]),35:(4,[18,26]),36:(7,[18]),37:(4,[6,26]),38:(1,[32]),39:(9,[9,19]),40:(8,[5,17]),
41:(6,[16,17]),42:(3,[17]),43:(5,[17]),44:(5,[8,17]),45:(4,[8,17]),46:(7,[31,32]),47:(7,[13,32]),48:(7,[17]),49:(2,[17]),50:(6,[25]),
51:(4,[33,35]),52:(6,[11]),53:(5,[23,33]),54:(9,[1,3,20]),55:(4,[35]),56:(7,[17,25]),57:(6,[36]),58:(4,[36]),59:(5,[30,33]),60:(9,[16])}
SPACE={
1:(5,[1,2,3,4,5,11,12]),2:(10,[19,20]),3:(10,[13,14,16,17]),4:(10,[19,22,24]),5:(10,[5,9,10,12]),6:(10,[17,19,23]),7:(10,[19,20,21]),8:(10,[19,21]),9:(10,[30,33,36]),10:(10,[19,22]),
11:(10,[5,6,10]),12:(10,[13,16,17]),13:(8,[17,19]),14:(5,[35]),15:(5,[13,14]),16:(8,[20]),17:(8,[17,24]),18:(6,[13,15,16]),19:(3,[31,36]),20:(3,[31,36]),
21:(4,[19,21]),22:(8,[10,24]),23:(5,[13,17]),24:(6,[13,15]),25:(1,[1]),26:(5,[4,8,27]),27:(4,[30,32,34]),28:(2,[30,36]),29:(10,[19,21]),30:(10,[19,20]),
31:(10,[14,19]),32:(10,[5,7,10]),33:(10,[17,23]),34:(8,[19,21]),35:(8,[13,17,26]),36:(8,[13,16,17]),37:(10,[14,16,24]),38:(10,[19,20]),39:(10,[4,8,9]),40:(10,[13,16,17]),
41:(10,[19,21]),42:(10,[19,20]),43:(10,[5,7,11]),44:(10,[13,16,17]),45:(10,[19,21]),46:(10,[19,21]),47:(10,[13,16]),48:(8,[19,20]),49:(2,[31,36]),50:(3,[33,36]),
51:(8,[13,16]),52:(6,[4,8,33]),53:(6,[13,26]),54:(4,[30,33]),55:(5,[2,10,24]),56:(4,[35]),57:(3,[27,30]),58:(3,[24]),59:(8,[19,21]),60:(8,[19,21])}

def build_npc_catalog(prefix, records, rules):
    family=rules['family'];specs=WUXIA if family=='wuxia' else SPACE
    skills={s['id']:s for s in rules['skills']}
    opponents={o['character_source_key']:o for o in rules['opponents'] if o.get('character_source_key')}
    result=[];evidence=[]
    for source in records:
        if not re.fullmatch(r'P\d{2}',source['source_id']):continue
        number=int(source['source_id'][1:]);rank,known=specs[number]
        known=[f'{prefix}-s{n:02}' for n in known]
        # Empty inventories are deliberate: e.g. an untrained novice has no
        # advanced martial skill merely because their temple teaches one.
        rank=max([rank]+[skills[k]['required_rank'] for k in known])
        health=100+(rank-1)*25;energy=100+(rank-1)*10
        boss=opponents.get(source['id'])
        if boss:
            rank=max(rank,boss.get('rank',1));health=boss['health'];energy=boss.get('energy',100)
            known=list(dict.fromkeys(known+boss.get('skill_ids',[])))
        actor=dict(family=family,rank=rank,experience=0,health=health,max_health=health,energy=energy,max_energy=energy,injury=0,currency=0,skills={s:min(4 if family=='wuxia' else 10,max(1,rank//2)) for s in known})
        if boss and boss.get('inventory'):actor['inventory']=boss['inventory']
        # Only explicit personal study is scheduled. Teaching, commerce and
        # an aspiration to learn do not silently grant personal training time.
        training={41:17} if family=='wuxia' else {15:14,35:13}
        selected=f'{prefix}-s{training[number]:02}' if number in training else ''
        plan=dict(action='train' if selected else 'none',training_percent=25 if selected else 0,rank_limit=rank)
        if selected:plan['skill_id']=selected
        result.append(dict(character_source_key=source['id'],actor=actor,plan=plan))
        evidence.append(dict(id=source['id'],name=source['name'],rank=rank,skills=known,plan=plan,source=source['content'],basis='Authored adaptation of manuscript vocation and period-specific strength; numerical values are implementation parameters.',opponent_id=boss['id'] if boss else None))
    return result,evidence
