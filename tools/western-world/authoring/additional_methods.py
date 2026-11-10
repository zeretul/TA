"""Optional methods for three established skills; no new rules engine."""
import copy

def add_methods(definitions,english,rules):
 byid={d['id']:d for d in definitions};skills={s['id']:s for s in rules['skills']}
 def variant(task,source,id,skill,title,ten,body,ben,success,sen,echo,een):
  d=byid[task];c=copy.deepcopy(next(x for x in d['choices'] if x['id']==source));old=english[task]['choices'][source]
  c.update(id=id,skill_id=skill,required_skill=skill,required_skill_rank=1,use_skill_id=skill,title=title,description=body,success=success,success_echo=echo)
  c['success_reaction']['statement']=echo
  c['required_items']={skills[skill]['required_tool']:1} if skills[skill].get('required_tool') else {}
  e=copy.deepcopy(old);e.update(title=ten,description=ben,success=sen,success_echo=een,success_reaction=een)
  d['choices'].append(c);english[task]['choices'][id]=e
  return c
 c=variant('sg-q14','sg-q14.c1','sg-q14.c3','sg-listen','听经手人对照收条讲述','Hear the handler compare the receipts','在旧货商在场时对照收条听清两段交接，只记录真正听见的内容。','With the trader present, compare the slips with two accounts of the transfer, recording only what is actually heard.','两段交接用了不同的“替人收好”说法；你指出差异，尚不能因此认定赠与或偷窃。','The accounts use different meanings of “keep it for someone.” You identify the difference without declaring a gift or theft.','旧货商请持有人核对原话，铜钮仍按既有保管安排放着。','The trader asks the holder to check the words; the button remains under its existing custody.')
 c['duration_seconds']=1800
 c=variant('sg-q23','sg-q23.c1','sg-q23.c3','sg-legacy','把旧档放回当年住户的经历中','Read the archives against residents’ lives','以已学遗产释读核对三代住户口述与旧档；书记在场提供已获准查阅的页，不把族谱当产权判决。','Use learned legacy interpretation to compare three generations of accounts with pages the present archivist may show. Genealogy is no property judgment.','你分开了“受托保管”与“世代所有”两种记法，整理出可带走的核验副本；继承争议仍没有被替双方裁定。','You distinguish custody from inherited ownership and prepare an authorized checked copy, without adjudicating the dispute for either party.','书记把两种记法分别标注，街坊可以继续讨论现实维护，不必先选一位君主。','The archivist labels both usages separately. Neighbors can discuss present maintenance without first choosing a monarch.')
 c.update(duration_seconds=7200,difficulty=20)
 for reward in c['rewards']:
  if reward['kind']=='experience':reward['quantity']=32
 c=variant('sg-crypt-inquiry','sg-crypt-inquiry.stage2.c1','sg-crypt-inquiry.stage2.c3','sg-lockwork','以蜡留样后拆开外闸锁簧','Take a wax impression and release the outer latch','在乔文看守下用自己的细簧工具与一份蜡处理外闸，令点名机关脱开；不擅入内墓，也不虚构已经击败誓卫。','Under Joven’s supervision, use owned lock tools and one wax portion on the outer latch to disengage the muster, without entering inner graves or claiming combat victory.','外闸断开，誓卫停在旧线后。乔文取下旧闸钥交你核验，铁壳和名册均保留在原处。','The outer latch disconnects and the warden stops behind its line. Joven hands you the removed key for checking; shell and roll stay intact.','乔文给外闸另加警示，后来谁要重接机关仍须说明来意。','Joven marks the disengaged latch. Anyone reconnecting it later still needs to explain why.')
 c.update(item_costs={'sg-wax':1},duration_seconds=5400,difficulty=3)
