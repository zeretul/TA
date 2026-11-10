"""Distinct bounded failure consequences; no invented irreversible NPC decisions."""
FAILURES=[
('铁销尺寸与桥孔仍不合，桥面暂时沿原有限载使用，不能把这段工程记为修复完成。','The pin still does not fit the bridge socket. The existing load limit remains; this section cannot be recorded as repaired.','桥工把未完成处标在工单上，下一次得带尺寸或更可靠的工法来。','The crew marks the unfinished section. A later attempt needs measurements or a more reliable method.'),
('两份面粉记录仍对不上，热汤摊没有因此得到一笔不存在的清账。','The flour records still disagree. The soup stall receives no fictional debt clearance.','掌柜保留原单据，只愿按后来核实的数目继续谈。','The proprietor retains the original slips and will discuss figures that can later be checked.'),
('印记相似不足以说明两张税票来自同一次收费，你没有注销其中任何一张。','Matching marks do not establish that the tax slips describe one payment; neither is canceled.','核票员把疑票单独留存，车主仍需拿出原始经手凭据。','The clerk separates the disputed slips; their holders still need original handling evidence.'),
('水流和木排位置还不足以证明鱼道畅通，试运安排没有被确认。','The current and timber positions do not establish an open fish channel, so no trial run is confirmed.','渔户继续看水，放木者也没有获得可以任意占河的许可。','Fishers keep watching the water; timber handlers gain no unrestricted claim on the river.'),
('闸口仍漏水，田间轮放需要重新测量；这一回没有分出谁应优先用水。','The sluice still leaks. Rotation requires new measurements, and this attempt assigns no water priority.','农户按已有安排取水，争议没有被一个修复宣称掩盖。','Farmers use the existing arrangement; a repair claim does not conceal the dispute.'),
('旧村名还不能对应到一个可核实的新住处，信仍未被拆开或擅自转交。','The old village name cannot yet be matched to a verified address. The letter stays sealed and undelivered.','驿站把缺失的地名线索记下，等有依据再决定往哪送。','The station notes the missing location evidence and awaits a sound destination.'),
('冬粮人数与储量不能相互印证，这一轮没有形成新的借粮约定。','Winter headcounts and stores do not corroborate each other; no new grain loan is agreed.','军需书记继续保留两份账，不把短缺转成任何人的终身效忠。','The quartermaster retains both ledgers without converting a shortage into lifelong allegiance.'),
('练习中的停手信号仍未被可靠执行，教头中止这一轮，不把逞强算作学会。','The stop signal is not followed reliably. The instructor ends this round rather than counting bravado as mastery.','下一次练习要先把规则和余力说明白，旁观者的催促不算同意。','A later practice needs clear rules and adequate reserves; spectators cannot supply consent.'),
('证言与衣物上的线索仍有出入，没有新增死者身份或替家属写出他们未说的话。','Accounts and clothing evidence still differ. No deceased identity or unspoken family testimony is invented.','葬仪师保留空名栏，等到可以说明来源的新材料。','The officiant leaves the name blank pending evidence with a stated source.'),
('桩穴与足印不能确定旧界线，临时草场的使用条件也还没谈妥。','Postholes and tracks do not establish the old boundary, and temporary grazing terms remain unsettled.','牧人继续看管自己的牲畜，没有谁凭这一轮取得整片草地。','Herders continue tending their animals; nobody acquires the whole pasture from this attempt.'),
('水声在几处互相重叠，你还不能说明哪一声来自钟铜。','Water sounds overlap in several places; you cannot yet identify a bronze bell as the source.','林口的人保留各自听到的描述，下一次需有能区分声源的观察。','People at the verge retain their accounts. A later observation must distinguish the sources.'),
('根部与桥台的受力关系仍没查清，临时尝试没有让活根或石桥恢复可靠承重。','The load shared by roots and masonry remains unclear; neither gains reliable strength from the attempt.','根桥匠继续标示薄弱处，不把保树与保桥说成只能选一个。','The bridgewright keeps weak points marked without declaring that either tree or bridge must be sacrificed.'),
('样本来源或对照不足，这一回不能据此指认泉水污染者。','The sample provenance or controls are insufficient to identify a polluter.','守泉者保留疑问而非散布罪名；后来的检验需要清楚的取样记录。','The keeper retains the question rather than an accusation; later tests require clear sampling records.'),
('铜钮的转手链仍缺一段，物主与买主的权利没有因猜测改变。','One link in the button’s ownership chain remains missing. Guesses change neither party’s rights.','旧货商暂留相关收条，谁愿补充证据仍由本人决定。','The trader retains the slips; anyone supplying further evidence chooses to do so.'),
('栈道的沉陷还超出这次检查和补料能确认的范围，不能宣布整条沼路安全。','The walkway’s subsidence exceeds what this inspection and material can establish; the whole marsh route is not declared safe.','危险段仍有标记，打捞人要另找落脚处而不是被迫赌命。','Danger marks remain; salvagers must find other footing rather than be compelled to risk their lives.'),
('泵后的水仍混浊，这批处理没有达到可以供病房使用的条件。','Water remains cloudy after the pump. This batch is not cleared for the ward.','配水师分开保留有问题的水样，清水仍按现有来源调配。','The water engineer separates suspect samples; clean supplies still come from established sources.'),
('风轮转动仍不稳定，不能把井下通风已经恢复当作完工依据。','The fan still turns unevenly. Restored underground ventilation cannot be claimed as a completion fact.','当班工头记录停机处，后续进井须重新确认风况。','The shift foreman records the fault; air must be checked again before later entry.'),
('修件的接合仍不牢，工坊没有把它交给下一位使用者，也未支付完工款。','The repaired joint remains insecure. The workshop does not hand it to a user or pay completion wages.','铁匠把问题留在验件记录里；之后要以改进后的工法重新处理。','The smith keeps the defect on the inspection record for a better-prepared later attempt.'),
('封湖标线与水面实测仍有偏差，尚不足以形成双方认可的范围。','The lake boundary and measured waterline still differ, preventing an agreed perimeter.','渔户与学者继续保存各自依据，眼前并未新增赶人或禁航许可。','Fishers and scholars retain their evidence; no new eviction or navigation ban is created.'),
('观测日期与抄本版本没有对齐，这次核对不能宣布所有船图已经正确。','Observation dates and copy versions do not align. This check cannot certify every chart.','抄写员把可疑页另作标记，未收到更正的船不会被算作已经知情。','The copyist flags suspect pages; vessels without corrections are not treated as informed.'),
('纸张与课程的安排还没形成双方愿意接受的条件，捐赠并未成交。','Paper and lesson arrangements do not reach mutually acceptable terms; no donation is completed.','书院继续用现有纸本，不会为一笔未落实的捐赠改写入学资格。','The school uses its existing stock without changing admission for an unrealized donation.'),
('门缝的处理或纪念牌的约定仍不充分，居民没有得到一个已经修好的出入口。','The gap repair or plaque arrangement remains inadequate; residents have not gained a finished entrance.','街坊把通行问题和名号争论分开记下，下一轮仍需实际检查。','Neighbors record access problems separately from titles, with a real inspection still needed.'),
('两份继承书的保管链仍有空白，不能据此替任何一方确立王位或土地权。','Both inheritance documents have gaps in custody; neither crown nor land rights are established.','档案员保留版本差异，下一条证言必须说明本人实际见过什么。','The archivist preserves version differences; new witnesses must state what they actually saw.'),
('灯心的导接或回声抄录还不能稳定复核，古灯没有因这次尝试恢复完整功能。','The core connection or echo transcript cannot be reliably checked. The beacon has not regained full function.','守灯人继续按现有限制值守，缺失的记录没有被补写成远古真相。','The keeper works within existing limits; missing records do not become invented ancient truths.'),
('名单中的同音和转述仍未分清，不能把一个相似名字当作本人同意联系。','Similar names and secondhand accounts remain unresolved. A resemblance is not consent to contact.','中转人保留未确认栏，不把住处交给未经核实的寻访者。','The intermediary keeps entries unconfirmed and withholds addresses from unverified seekers.'),
('下一轮修缮的参与者或费用仍没说清，小车主没有因此承担新工期。','Participants or costs for the next repair remain unclear; small wagon owners take on no new schedule.','桥工保留未谈妥的部分，已经完成的旧工程不会被反复计作新报酬。','The crew keeps unresolved terms open without paying again for the completed earlier work.'),
('已有水样还不能排除这次争议的出水口，新的改造安排尚未成立。','Existing samples cannot rule out the disputed outlet; no new modification plan is agreed.','工人与守林者继续保留原记录，下一次需要不同来源的对照。','Workers and keepers retain the records; a later attempt needs an independent comparison.'),
('用图者收到的版本仍无法逐一确认，这次没有证明警示已送到所有船上。','The copies received by chart users remain unverified. The warning is not proven to have reached every vessel.','港边继续列出未核对的抄本，后来实际收到回信才会改变记录。','The dock keeps unchecked copies listed until actual replies change the record.'),
('守夜与维护的承诺尚未落实到自愿参加的人，街坊没有被统一编进新轮值。','Watch and maintenance commitments lack willing named participants; no blanket rota is imposed.','已修的门仍照常使用，后续合作要从未说清的责任重新谈。','The repaired gate remains in use, while cooperation requires clarifying the remaining duties.'),
('这一趟的能见记录不足，不能把未观测海面也算进古灯照明范围。','This trip’s visibility records are insufficient; unobserved waters are not added to the beacon’s reach.','小船户保留天气与遮挡疑问，不因一个乐观估计放弃备用岸标。','Boat owners retain weather and obstruction concerns and keep backup shore markers.'),
('当事人还没有确认愿意转告的内容，你没有替他们答应返乡或透露住处。','The person has not confirmed what may be relayed. You promise neither their return nor their address.','中转者保留这条未获许可的草稿，是否继续仍由本人选择。','The intermediary retains an unapproved draft; the person decides whether to continue.'),
]

def apply_outcomes(definitions,english):
 for d in definitions:
  if not d['id'].startswith('sg-q'):continue
  failure,fen,echo,een=FAILURES[int(d['id'][4:])-1]
  for c in d['choices']:
   c['failure']=failure;c['failure_echo']=echo;c['failure_reaction']['statement']=echo
   e=english[d['id']]['choices'][c['id']];e.update(failure=fen,failure_echo=een,failure_reaction=een)
 guilds={
 'white-thorn':('你逐件核对这趟已同意护送的补给，把缺的备用绑带列回军需单；这批物资仍属于团部。','You check the agreed escort’s supplies and return missing spare ties to the requisition. The stores remain the company’s property.','下一趟护送者可以据清单领物，是否出行仍要本人答应。','The next escort can draw supplies from the list; each member still chooses whether to travel.'),
 'nine-towers':('你把获准公开的抄本按版次重排，封存私档仍留在原柜，借阅者能分清修订前后。','You sort approved public copies by edition while sealed records stay in their cabinet, making revisions distinguishable.','借阅者开始指出另一页不同的注记，学社需实际核对后才能改抄。','Readers flag a different annotation that the school must check before copying changes.'),
 'ashriver-league':('公秤用已有标准砝码校准，摊主留下这一轮的刻记，不涉及任何旧欠款自动减免。','The shared scale is calibrated with existing weights. Traders record this check without automatically canceling old debts.','商贩下一次争论重量时有了可查记录，价钱仍由各自商议。','Traders have a checkable record for later weight disputes while prices remain negotiable.'),
 'deephearth':('你按约定修牢公共药架，标出受潮的一层留待晾干；药品仍由原负责者登记发放。','You secure the shared medicine shelf and mark a damp tier for drying; the existing custodian still logs distribution.','换班工人按新标记取放物件，后续添药仍须有真实来货。','Later shifts use the new markings; replenishment still requires an actual delivery.'),
 'lostlands':('获准转告的联系请求被分开封存，未获许可的住处没有抄进给寻访者的信。','Approved contact requests are separately sealed. Unapproved addresses stay out of letters to searchers.','中转人按本人允许的方式安排后续，有人只愿报平安而不愿见面。','The intermediary follows each person’s permitted contact method; some offer reassurance but decline a meeting.'),
 }
 for slug,(success,sen,echo,een) in guilds.items():
  d=next(x for x in definitions if x['id']=='sg-guild-'+slug);c=d['choices'][0];c['success']=success;c['success_echo']=echo;c['success_reaction']['statement']=echo
  e=english[d['id']]['choices'][c['id']];e.update(success=sen,success_echo=een,success_reaction=een)
