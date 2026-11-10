"""Bounded fantasy mechanics using the existing adventure journal and reducer."""
# slug, name, English, kind, power, cost, required rank, hours, school, description, EN
SKILLS = [
('blade','行路剑术','Wayfarer Blade','attack',10,4,1,2,'martial','踏实的步伐配合短促直刺，能护身也懂留出退路。','Short thrusts and sound footwork protect the traveler while leaving an exit.'),
('shield','白棘架盾','White Thorn Guard','guard',18,6,1,3,'martial','盾缘不追敌刃，先护住身体，再等同伴退到身后。','The shield protects the body before chasing blades, buying companions room to withdraw.'),
('riposte','侧锋还击','Side Edge Riposte','attack',20,8,3,5,'martial','让开正锋后从侧面还击，靠判断空隙而非一味猛攻。','Yield the direct line and answer through the opening rather than meeting force head-on.'),
('halt','截步压制','Checking Step','control',1,10,3,5,'martial','压住对手换脚的刹那，使其一次行动受阻。','Catch the moment an opponent shifts footing to disrupt one response.'),
('brace','断旗坚守','Brokenbanner Stand','guard',30,12,6,8,'martial','在狭处把呼吸与架势稳住，保护眼前这一轮交锋。','Set breath and stance in a narrow place to withstand the present exchange.'),
('spear','长锋突进','Longpoint Advance','attack',32,14,8,10,'martial','顺着长柄送出一线直锋，进身时必须看清退路。','Advance along a long weapon’s line while keeping the retreat visible.'),
('banner','护旗反震','Banner Rebound','guard',24,16,12,14,'martial','把实际来势沿盾面引回；精神冲击不能折返，对手高出自己两级及以上时会反噬自身。','Return actual incoming physical force along the shield. Mental damage cannot be reflected; an opponent two or more ranks higher causes backlash instead.'),
('oath-edge','誓痕破阵','Oathscar Break','attack',48,22,18,22,'martial','多年练成的破守一击，誓言是来历，不是免于受伤的护符。','A seasoned guard-breaking stroke; its oath is a history, not immunity from harm.'),
('spark','烛焰引锋','Candleflame Edge','attack',14,7,1,3,'arcane','以炭粉与手中导具聚起短促火光，沿既有攻势送出。','Charcoal and a carried focus gather a brief flame along the wielder’s attack.'),
('ward','石纹护幕','Stonepattern Ward','guard',20,8,1,4,'arcane','用炭粉补齐导石上的护纹，撑过眼前的交锋后便渐渐散去。','Charcoal completes the focus pattern, sustaining a ward for the present exchange before fading.'),
('frost','薄霜缠足','Rime Snare','control',1,11,3,6,'arcane','冷雾附在脚边，扰乱一次换位；对方挣脱后会有所防备。','Rime disrupts one change of footing; a recovering opponent is prepared for another attempt.'),
('static','鸣弦束刃','Humming Bind','attack',24,12,5,8,'arcane','以鸣响的符线约束导具上的力量，短距释放。','A humming rune line shapes a short release through the focus.'),
('seal','闭纹禁咒','Closed Sigil','control',1,16,8,12,'arcane','闭纹扰乱有心智对手的一次行动，并造成有限心神冲击；不能解除已成立的术式，也不留下精神侵蚀。','A closed sigil disrupts one action of a conscious opponent and deals bounded mental damage. It neither dispels existing spells nor causes lasting corruption.'),
('mirror','镜纹返流','Mirrorflow','guard',30,18,12,16,'arcane','护纹折回部分实际来势，不能折返精神冲击；对手高出自己两级及以上时会反噬，且每次仍须支付炭料与精力。','A ward returns part of actual incoming physical force, never mental damage. An opponent two or more ranks higher causes backlash; every use still spends charcoal and effort.'),
('starfire','星屑引锋','Starshard Edge','attack',42,21,16,20,'arcane','磨细的辉晶沿器身发亮，既消耗材料，也需要稳定精神。','Ground glowstone lights the focus, consuming material and concentration alike.'),
('beacon','古灯续焰','Ancient Beacon Flame','attack',55,28,22,28,'arcane','从古灯修护笔记发展出的战斗术，不包含唤醒所有灯塔的权能。','A combat art derived from beacon maintenance, not authority to awaken every tower.'),
('trail','荒径辨踪','Trailreading','social',8,3,1,2,'wild','辨足印、折枝与天气留下的痕迹，先分清新旧。','Separate fresh tracks from old marks in mud, branches and weather.'),
('bow','猎手定弦','Hunter’s Draw','attack',14,6,1,3,'wild','等呼吸平稳再放箭，每次使用都要有真正的箭。','Loose on a steady breath, with a real arrow for every shot.'),
('quietstep','苔径轻行','Mossstep','pilot',0,7,2,4,'wild','选择稳定落脚点，为追赶或脱离争取距离，而不是瞬移。','Choose firm footing to gain distance in pursuit or withdrawal, never teleport.'),
('bindshot','缚索射击','Corded Shot','control',1,10,4,6,'wild','把细索送到目标脚边，不保证任何体型都被永久捆住。','Send a cord toward the target’s footing; no size is permanently bound by default.'),
('herbwise','山野辨药','Field Herbcraft','engineering',12,4,1,3,'wild','辨识可用药草并避开近似毒种，制作仍需配方与时间。','Identify useful herbs and poisonous lookalikes; preparation still requires a recipe and time.'),
('screen','林隙掩护','Glade Screen','guard',24,10,6,8,'wild','借枝影与身位减轻来势，不让树林凭空替你挡下一切。','Use cover and position to soften an attack without making the forest an invincible shield.'),
('longshot','风向长射','Windread Shot','attack',35,16,10,14,'wild','按近处风势调整射线，远方未见的敌人不因此变得可知。','Adjust to observed wind without revealing unseen distant enemies.'),
('pathkeeper','守径连射','Pathkeeper Volley','attack',46,22,18,22,'wild','一束紧凑的连射封住小段来路，耗箭与精力都不轻。','A compact volley covers a short approach at a substantial cost in arrows and effort.'),
('listen','门缝听声','Doorway Listening','social',8,3,1,2,'shadow','听脚步和谈话片段，记清自己真正听见了什么。','Listen for footsteps and fragments, recording only what was actually heard.'),
('knife','袖边短刃','Sleeveknife','attack',12,4,1,3,'shadow','短刃贴近身体，快进快退；不是必定致命的暗杀按钮。','Keep the blade close and move quickly; this is no guaranteed assassination.'),
('lockwork','细簧开锁','Fine Spring Lockwork','engineering',10,4,2,4,'shadow','用可回收的细簧工具判断锁芯，不能替代门后真实的准入风险。','Read a lock with reusable spring tools; opening it does not erase the risk beyond.'),
('smoketurn','烟幕错步','Smoke Turn','pilot',0,8,3,5,'shadow','借一撮烟粉扰乱视线并移动，烟散后仍留下痕迹。','Spend smoke powder to obscure movement; traces remain after it clears.'),
('nerve','扣腕截势','Wrist Check','control',1,9,5,8,'shadow','近身扣住发力的手腕，强敌的恢复与反击仍然成立。','Interrupt a wrist at close range without denying a stronger foe recovery and response.'),
('venom','夜苔涂刃','Nightmoss Edge','poison',12,10,7,10,'shadow','把有限毒膏用于刃口，敌人与自己都不能靠一句话免疫毒性。','Apply a finite toxic paste; neither side becomes immune merely by saying so.'),
('feint','双影试探','Two Shadow Feint','attack',32,14,11,14,'shadow','以假动作逼出防守，真正攻击仍受装备和伤势影响。','Draw a guard with a feint; the real strike still depends on equipment and injury.'),
('unseenroad','无灯退路','Unlit Retreat','pilot',0,18,17,20,'shadow','熟悉黑暗中的落脚与遮蔽，争取脱离但不保证无伤消失。','Use footing and cover in darkness to seek withdrawal, not a guaranteed harmless disappearance.'),
('firstaid','行旅包扎','Traveler’s Dressing','heal',16,5,1,2,'faith','清理伤口、稳住绷带，先保住还能做的事。','Clean and bind a wound, preserving what the injured person can still do.'),
('prayer','守夜祷光','Vigil Light','guard',18,7,1,3,'faith','烛光与短祷帮助稳定架势，信仰不替代真实防护。','A candle and short prayer steady the stance without replacing real protection.'),
('purge','清盐净伤','Cleansalt Remedy','antidote',24,9,3,5,'faith','以净盐与药草减轻毒性，不能逆转所有伤病。','Salt and herbs reduce poisoning, not every illness or injury.'),
('mend','余烛缝合','Last Candle Mending','heal',28,12,5,8,'faith','把有限的暖光聚在伤处，消耗药布与施术者精力。','Gather limited warmth at a wound, spending dressings and the healer’s effort.'),
('resolve','誓心定念','Steady Vow','guard',22,11,7,10,'faith','记住愿意保护的具体人，稳住架势并减轻当轮心神冲击；不能替你驱散术式或免除行动受阻。','Remember someone worth protecting, steady your stance and reduce mental damage in this exchange. It neither dispels a spell nor prevents a separate interruption.'),
('censure','晨钟斥刃','Dawn Bell Rebuke','attack',30,15,10,14,'faith','钟音随导具一同震出，抵抗伤害的能力不随信仰标签判定。','A bell note travels through the focus; protection is not decided by a religious label.'),
('sanctuary','守门圣幕','Threshold Sanctuary','guard',38,18,15,20,'faith','守住身前有限的一片空间，不能宣布一整座城绝对安全。','Protect a small space before the caster, not declare an entire city safe.'),
('restoration','长夜回暖','Longnight Warmth','heal',48,24,21,26,'faith','经过漫长学习的疗护术，仍需要材料与实际存活的伤者。','A hard-learned healing art requiring supplies and a living recipient.'),
('greenhand','青枝调息','Greenbranch Breath','energy',12,2,1,3,'druid','以清水与草香安定呼吸，恢复的只是有限精力。','Water and herbs calm breathing, restoring only a bounded amount of effort.'),
('thorn','棘刺引锋','Thorn Edge','attack',16,7,1,3,'druid','把种囊里的细根催成棘刺，沿导具送出一记短击。','Coax the packet’s roots into thorns for one short strike along the focus.'),
('rootbind','根须缠步','Rootstep Bind','control',1,11,3,6,'druid','让携带的根种短暂生长，缠住一次换位便会失力。','Carried root seeds briefly grow to catch one step before losing strength.'),
('bark','树皮护纹','Bark Ward','guard',26,11,5,8,'druid','树脂与碎皮结成短时护纹，不把身体永久变为树。','Resin and bark form a brief ward without permanently changing the body into a tree.'),
('sap','露汁疗护','Dewsap Care','heal',27,12,7,10,'druid','药汁配合自然法术，伤口恢复不能跳过材料。','Herbal sap accompanies natural magic; healing does not skip supplies.'),
('spores','沉钟孢尘','Bellwood Spores','poison',18,14,10,14,'druid','使用封好的一小包孢粉，周围有无辜者时不应随意施放。','Use one sealed packet of spore dust, with care for nearby bystanders.'),
('oldroot','古根坚壁','Oldroot Bulwark','guard',38,19,16,20,'druid','将根纹沿防具展开，守住一次足够漫长的冲击。','Spread root patterns along protection to endure a sustained exchange.'),
('wildreturn','荒原回生','Wildland Renewal','heal',45,25,22,28,'druid','把多种药材的效力接续起来，不能使死者或毁林凭空复原。','Sequence several remedies without resurrecting the dead or instantly restoring a ruined forest.'),
('forge','冷锤锻造','Coldhammer Smithing','engineering',12,4,1,3,'craft','控制火色与锤落，先把一枚可靠钉子做好。','Control heat and hammerfall, beginning with one reliable nail.'),
('leatherwork','补缀制革','Patchwork Leathercraft','engineering',10,4,1,3,'craft','辨磨损方向再下针，旧皮也能修成合用护具。','Read wear before stitching; old leather can become serviceable protection.'),
('alchemy','旅途炼剂','Roadside Alchemy','engineering',14,5,2,5,'craft','量取、过滤与试验都要耐心，不靠颜色猜药效。','Measure, filter and test patiently rather than judging a remedy by color.'),
('runes','导纹镌刻','Focus Engraving','engineering',16,6,4,7,'craft','把可验证的纹路刻入器具，未试过的图样只是方案。','Engrave tested patterns; an untested design remains a proposal.'),
('fieldrepair','营地整备','Camp Repair','engineering',0,5,2,4,'craft','用自己的锻修具整备扣件与把手；实际修件工作按材料、时间与精力结算，不凭空复制装备。','Use owned forge tools on buckles and handles. Actual repair jobs settle materials, time and effort without duplicating equipment.'),
('glasswork','盐晶熔制','Saltglass Work','engineering',20,7,7,10,'craft','掌握冷却速度，把裂开的材料留在废料盘里。','Control cooling and keep cracked pieces in the scrap tray.'),
('masterforge','双炉精锻','Twin Furnace Forging','engineering',26,9,12,16,'craft','在两种炉温间交替加工，成品依然需要真实金属。','Alternate two furnace temperatures, still using real metal.'),
('beaconwork','古灯修护','Beacon Restoration','engineering',32,12,18,24,'craft','逐一核对旧结构与新部件，不把不了解的残片强行装回。','Compare old structures with replacements rather than forcing unknown fragments into place.'),
('parley','桥边说合','Bridge Parley','social',10,3,1,2,'social','先让双方说清眼前所求，说服并不控制他人意志。','Clarify each side’s immediate need; persuasion is not mind control.'),
('ledger','旧票核账','Receipt Reading','social',10,3,1,3,'social','比较数量、日期与经手人，不在缺页处编出证据。','Compare amounts, dates and handlers without inventing missing evidence.'),
('survey','野外测绘','Field Survey','social',12,4,2,4,'social','标出实际走过的路和看见的危险，不把空白画成安全。','Mark traveled routes and observed hazards; blank space does not mean safety.'),
('lore','旧文辨读','Old Script Reading','social',14,5,4,6,'social','先辨抄写年代与用语，再尝试理解作者的意思。','Date the copy and its language before interpreting its author.'),
('song','炉边叙事','Fireside Storycraft','social',14,5,3,5,'social','用人们认得的细节讲故事，不把听众当可收割声望的道具。','Tell stories through recognizable details, not as a machine for harvesting reputation.'),
('witness','证言比对','Witness Comparison','social',18,6,7,10,'social','保留彼此矛盾的说法，直到新的依据足以澄清。','Preserve conflicting accounts until further evidence can clarify them.'),
('mediation','五席调停','Five Seat Mediation','social',24,8,12,16,'social','把争议拆成可逐步处理的事，不能替未到场者承诺。','Break disputes into workable parts without making promises for absent people.'),
('legacy','遗产释读','Legacy Interpretation','social',30,10,18,22,'social','将旧档、口述与眼前生活连起来，允许留下尚未解决的问题。','Connect archives, testimony and present lives while leaving unanswered questions open.')]
# id, zh, en, kind, price, power, description, English description
ITEMS = [
('travel-blade','行旅短剑','Travel Sword','weapon',30,8,'剑柄缠着耐磨麻绳，缺口已经磨平。','A hemp-wrapped grip and carefully ground old nicks.'),
('thorn-sword','白棘长剑','White Thorn Longsword','weapon',110,16,'剑格有白棘纹，铸造者的印比团徽更小。','A white thorn marks the guard above a smaller smith’s stamp.'),
('ash-spear','灰木长枪','Ashwood Spear','weapon',75,13,'长柄经火烘直，尾端装有防潮铁箍。','A fire-straightened shaft carries a damp-resistant ferrule.'),
('shortbow','渡鸦短弓','Raven Shortbow','weapon',65,12,'弓身短而结实，适合林缘近处的视线。','A compact sturdy bow suited to clearings and woodland edges.'),
('longbow','风骨长弓','Windbone Longbow','weapon',180,23,'弓片留下缓慢上油的深色纹路。','Dark grain records years of careful oiling.'),
('dagger','细齿匕首','Finefang Dagger','weapon',45,9,'细窄刀刃便于近身，却不是开罐头的好工具。','A narrow close blade, poorly suited to opening cans.'),
('focus-staff','导纹木杖','Patterned Staff','weapon',80,14,'铜丝嵌入干木，顶端可绑更换的导石。','Copper wire runs through dry wood beneath a replaceable focus stone.'),
('prayer-mace','守夜锤','Vigil Mace','weapon',95,15,'柄头磨得光滑，常被守夜者拿来轻叩钟架。','Its polished head is often tapped against a vigil bell frame.'),
('rootstaff','老根枝杖','Oldroot Staff','weapon',120,17,'不是活树的心脏，只是一段许可取下的结实老枝。','A sturdy branch taken with permission, not a living tree’s heart.'),
('rune-sabre','嵌纹军刀','Inlaid Saber','weapon',360,29,'纹槽里有清理粉尘的细孔，维护比外观更费心。','Fine vents clear its inlaid grooves; maintenance matters more than ornament.'),
('glass-lance','盐晶锋枪','Saltglass Lance','weapon',520,36,'晶锋可以换新，断裂的旧件会被留下回炉。','The crystalline point is replaceable; broken parts return to the furnace.'),
('beacon-edge','灯心导刃','Beaconcore Edge','weapon',850,44,'淡光沿刃脊走动，护手上还留着修护者的测试划痕。','Pale light follows the spine beside a restorer’s test scratches.'),
('padded','补缀软甲','Patched Jack','armor',28,5,'麻布夹层里塞着旧羊毛，雨后必须晾干。','Old wool fills linen layers that need drying after rain.'),
('leather','油革护甲','Oiled Leather','armor',65,10,'接缝留有活动余量，肩部有一小块不同颜色的补皮。','Flexible seams and one mismatched shoulder patch.'),
('mail','旧铆环甲','Riveted Mail','armor',150,18,'几代铁匠修补过同一件甲，环的色泽并不一致。','Generations of repair left rings of different colors.'),
('thorn-mail','白棘衬甲','Thorn Lined Mail','armor',280,24,'内衬可拆洗，徽记不替穿戴者挡刀。','A washable lining; the badge itself stops no blade.'),
('rune-coat','导纹护衣','Patterned Coat','armor',330,25,'细线绣在内侧，避免在雨中直接磨损。','Fine patterns sit inside to avoid weathering.'),
('root-mail','根纹藤甲','Rootweave Armor','armor',390,28,'经过干燥定形的藤片以皮带相连。','Dried shaped vine plates join with leather straps.'),
('forgeplate','深炉板甲','Deephearth Plate','armor',620,34,'每片板都有工匠签记，内侧留着换垫的位置。','Each plate has a maker’s mark and replaceable padding.'),
('courtguard','旧庭守甲','Oldcourt Harness','armor',800,40,'王号早已磨掉，关键铆钉换成了新制品。','The royal name is gone; the essential rivets are new.'),
('bread','黑麦面包','Rye Bread','consumable',4,14,'切面粗实，适合掰成小块慢慢吃。','A dense loaf best eaten in small pieces.'),
('broth','浓蔬汤包','Vegetable Broth','consumable',8,24,'干菜与盐装在布袋里，热水能让它重新有香味。','Dried vegetables and salt regain their scent in hot water.'),
('tonic','苦草伤药','Bitterleaf Dressing','consumable',16,25,'有明显草苦味，蜡封上写着磨制日期。','Strong herbal bitterness under a dated wax seal.'),
('salve','清盐解剂','Cleansalt Antidote','consumable',18,28,'盐粒与药膏分开装，使用前才调匀。','Salt and salve are mixed only before use.'),
('rest-draught','安神草饮','Calming Infusion','consumable',22,35,'清淡草香压过微涩，不会把疲劳永久抹去。','A pale herbal scent over mild bitterness, never permanent freedom from fatigue.'),
('greater-tonic','修院浓药','Hospice Concentrate','consumable',48,55,'小瓶装着耐心熬滤的药液，封口系着用途标签。','Slow-filtered medicine in a small labeled bottle.'),
('trail-ration','山行粮包','Trail Rations','consumable',14,32,'烤谷、果干与少量盐分袋装好。','Toasted grain, dried fruit and salt packed separately.'),
('rescue-dose','回暖急救剂','Warming Rescue Dose','consumable',65,70,'为受伤者保温缓痛的急救药，不能给死者另造生命。','Emergency warmth and pain relief, not a new life for the dead.'),
('fire-flask','炭油投瓶','Charoil Flask','consumable',25,28,'陶瓶一经投碎便不能回收，瓶颈有醒目的红绳。','A red-tied clay bottle that cannot be recovered after breaking.'),
('spore-flask','孢尘封罐','Sealed Spore Jar','consumable',28,20,'双层封纸挡住细粉，不能放在食物旁。','Double paper seals hold powder away from food.'),
('iron','锻铁条','Forging Iron','material',10,0,'按炉批打了小点的铁条，便于查回杂质来历。','Iron bars dotted by batch so impurities can be traced.'),
('leather-strip','鞣皮料','Tanned Hide','material',8,0,'皮面有自然伤痕，切割前要避开薄弱处。','Natural scars require careful cutting.'),
('cloth','洁净药布','Clean Dressing Cloth','material',4,0,'晒干后折成小方，边缘拆了散线。','Sun-dried squares with loose edges removed.'),
('herbs','苦叶药草','Bitterleaf Herbs','material',5,0,'叶背银灰，茎被成束倒挂晾干。','Silver-backed leaves dried in hanging bunches.'),
('salt','净盐','Clean Salt','material',3,0,'干燥细盐装在纸筒里，怕潮也怕泼洒。','Fine salt in paper tubes, vulnerable to damp and spills.'),
('charcoal','细炭粉','Fine Charcoal','material',3,0,'布袋一碰就留下黑印，施法与炉作都能用。','The bag leaves black fingerprints; useful to both spellwork and furnaces.'),
('crystal','碎辉晶','Glowstone Shards','material',8,0,'不整齐的小晶片只发微光，需要逐一检查裂纹。','Dim irregular shards, each needing a crack check.'),
('resin','松脂块','Pine Resin','material',4,0,'冷时硬脆，掌心捂热后便有浓香。','Brittle when cold and fragrant when warmed.'),
('seed','根须种囊','Rootseed Packet','material',4,0,'种子包着薄土，只够短暂催生一小段根。','Seeds in thin soil sustain only a small brief root growth.'),
('smoke','烟粉包','Smoke Powder','material',6,0,'每包分量固定，受潮后会结块。','Measured packets that clump when damp.'),
('venom-paste','夜苔膏','Nightmoss Paste','material',9,0,'深绿膏体用独立木签取用，不宜沾皮。','Dark paste handled with a separate stick and kept off skin.'),
('arrow','制式箭','Fletched Arrow','material',2,0,'箭羽用不同色线标出工坊。','Colored thread identifies the fletching workshop.'),
('wax','净蜂蜡','Clean Beeswax','material',5,0,'浅黄蜡块带淡蜜香，可封口也可制烛。','Pale wax for seals and candles.'),
('silver-wire','细银丝','Fine Silver Wire','material',12,0,'卷在木轴上，折痕过深就需要裁去。','Wound on a wooden spool; deep kinks must be cut away.'),
('glass','盐晶片','Saltglass Plate','material',14,0,'透亮薄片四边已磨圆，尚未刻纹。','Clear rounded plates awaiting engraving.'),
('spirit','蒸馏酒精','Distilled Spirit','material',7,0,'密塞小瓶要远离炉火，不是能当水喝的烈酒。','A tightly stoppered bottle kept from fire, not a drinking ration.'),
('grain','干谷料','Dry Grain','material',2,0,'带一点麸皮，能磨粉也能熬粥。','Grain with some bran, for milling or porridge.'),
('wood','干燥灰木','Seasoned Ashwood','material',6,0,'木纹顺直，端面涂蜡防裂。','Straight grain with waxed ends against splitting.'),
('focus','便携导石','Portable Focus','tool',35,0,'掌心大小的刻纹石，可校正各种导具。','A palm-sized patterned stone used to align different implements.'),
('prayer-cord','祷结与小钟','Prayer Knot and Bell','tool',25,0,'绳结记录守夜次序，钟很轻，不惊扰病人。','Knots mark vigils and the light bell spares patients a shock.'),
('herb-kit','采药包','Herbalist Kit','tool',24,0,'小剪、布袋与标签各占一个夹层。','Shears, bags and labels each have a compartment.'),
('forge-tools','旅行锻修具','Travel Forge Tools','tool',40,0,'锤、钳、小砧与锥针各有固定的位置，皮边修补与冷锻都用得上。','Hammer, tongs, small anvil and awl have fitted slots, serving cold forging and leather-edge repairs.'),
('lockpicks','细簧工具卷','Spring Tool Roll','tool',32,0,'每根细簧各有标记，少一根很容易发现。','Each fine spring is marked, making losses visible.'),
('survey-kit','测绳与方板','Survey Cord and Board','tool',30,0,'结绳按实长校过，木板上有旧雨痕。','A calibrated cord and a rain-marked board.'),
('lantern','挡风提灯','Hooded Lantern','tool',18,0,'灯罩有滑动挡板，开合时留意烫手的边。','A sliding hood with edges that grow hot.'),
('rope','结实长绳','Stout Rope','tool',15,0,'中段补过一处，额定承重不能凭信心增加。','One repaired section; confidence does not increase its safe load.'),
('journal','空白旅记','Blank Travel Journal','tool',9,0,'纸页不算白，足够写清亲眼见到的事。','Rough paper sufficient for recording what one actually saw.'),
('old-seal','褪色路印','Faded Road Seal','tool',0,0,'不是通行万能令，只能指向曾经的签发者。','Not a universal pass, only a clue to its former issuer.'),
('bridge-pin','修桥铁销','Bridge Repair Pin','material',0,0,'按桥工给的尺寸制成，普通铁销并不能随意替换。','Made to the bridge crew’s measurements, not freely interchangeable.'),
('water-sample','封蜡水样','Waxed Water Sample','tool',0,0,'标着采样处和时刻，污染了便失去比对价值。','Labeled with place and time; contamination destroys its evidential value.'),
('archive-copy','核验档副','Checked Archive Copy','tool',0,0,'页边有两位抄写人的核对印，不代表内容没有争议。','Two copyists checked the margins, not settled every dispute within.'),
('family-token','缝布家徽','Cloth Family Token','tool',0,0,'用旧衣缝出的记号，不在金银价目表里。','A sign stitched from old clothes, outside the price list of gold and silver.'),
('shared-map','共绘路图','Shared Road Map','tool',0,0,'不同笔迹标着亲历路线，空白处仍旧空着。','Different hands mark traveled routes, leaving unknown places blank.')]

ITEMS += [('buckler','白木手盾','Whitewood Buckler','tool',26,0,'削薄的白木覆着皮沿，握带可以按手掌大小调节。','Thin whitewood with a leather rim and an adjustable handgrip.'),('oath-key','脱下的旧闸钥','Removed Sluice Key','tool',0,0,'倒旗墓道点名闸口拆下的旧钥，齿沿磨损，只对应那具旧装置。','A worn key removed from the crypt muster sluice, fitting only that old mechanism.'),('drowned-ledger','刮改运粮账','Altered Grain Ledger','tool',0,0,'从沉王水宫封匣里取出的原账，几行被刮去；旧印不能单独证明谁下令。','The original ledger from the sealed palace box has scraped entries; an old seal alone proves no order.') ]

GROUPS = {'martial':('白棘演武场','White Thorn Yard'), 'arcane':('渡烛书院','Candleford School'),'wild':('沉钟林口','Sunkenbell Verge'),'shadow':('黑蜡下街','Blackwax Understreet'),'faith':('春雪修院','Springsnow Hospice'),'druid':('根桥聚落','Rootbridge Enclave'),'craft':('冷锤工坊','Coldhammer Workshop'),'social':('渡鸦桥镇','Ravenbridge')}

def metadata(zh,en,bg,bgen,grade='green'):
 return {'quality_grade':grade,'description':zh,'background':bg,'localizations':{'en-US':{'description':en,'background':bgen}}}

def build_rules():
 skills=[];items=[];names={};definitions={}
 for slug,zh,en,kind,price,power,desc,den in ITEMS:
  ident='sg-'+slug; names[ident]=en
  item={'id':ident,'name':zh,'kind':kind,'scale':'person','price':price,'power':power,'acquisition':'market' if price else 'reward'}
  item.update(metadata(desc,den,'烬河与相邻诸境的工匠、农户和行旅者实际使用的物件，来源和持有会随本局经历留下记录。','Used by craftspeople, growers and travelers across Ashriver and its neighbors; provenance and ownership remain part of the current story.','white' if price<40 else 'green' if price<200 else 'blue' if price<600 else 'purple'))
  if kind in ('weapon','armor'):item['durability']=100
  if kind=='consumable':item['effect']='energy' if slug in ['bread','broth','rest-draught','trail-ration'] else 'antidote' if slug=='salve' else 'attack' if slug=='fire-flask' else 'poison' if slug=='spore-flask' else 'heal'
  if slug in ['shortbow','longbow']:item['materials']={'sg-arrow':1}
  if slug in ['family-token','oath-key','drowned-ledger']:item['unique']=True
  items.append(item);definitions[ident]={'name':en,'description':den,'background':item['localizations']['en-US']['background']}
 for index,(slug,zh,en,kind,power,cost,rank,hours,school,desc,den) in enumerate(SKILLS):
  ident='sg-'+slug; names[ident]=en;grade='green' if rank<6 else 'blue' if rank<15 else 'purple'
  skill={'id':ident,'name':zh,'kind':kind,'scale':'person','power':power,'cost':cost,'required_rank':rank,'training_hours':hours,'required_item':'sg-manual-'+slug}
  place,p_en=GROUPS[school];bg=f'{place}流传的实践方法。出身只影响最初会什么，后来可凭来源与实际学习兼修；它不授予整个行会的许可。';ben=f'A practiced method associated with {p_en}. Origins determine only initial knowledge; anyone may study it with a real source and training. It grants no organization-wide permission.'
  skill.update(metadata(desc,den,bg,ben,grade))
  if school in ['arcane','druid']:skill['required_tool']='sg-focus';skill['materials']={'sg-charcoal':1} if school=='arcane' else {'sg-seed':1}
  if school=='faith':skill['required_tool']='sg-prayer-cord';skill['materials']={'sg-cloth':1} if kind=='heal' else {'sg-salt':1}
  # The equipped bow supplies the ordinary arrow; only a volley spends extras.
  if slug=='pathkeeper':skill['materials']={'sg-arrow':2}
  if slug=='bindshot':skill['materials']={'sg-arrow':1}
  if slug in ['bow','longshot','pathkeeper','bindshot']:skill['required_tool']='sg-shortbow'
  if slug in ['quietstep','smoketurn','unseenroad']:skill['mobility']=12 if rank<3 else 20 if rank<10 else 32
  if slug=='starfire':skill['materials']={'sg-crystal':1}
  if slug=='smoketurn':skill['materials']={'sg-smoke':1}
  if slug=='venom':skill['materials']={'sg-venom-paste':1}
  if slug in ['shield','banner']:skill['required_tool']='sg-buckler'
  if slug=='lockwork':skill['required_tool']='sg-lockpicks'
  if slug in ['forge','fieldrepair','masterforge','glasswork','leatherwork']:skill['required_tool']='sg-forge-tools'
  if slug in ['beaconwork','runes']:skill['required_tool']='sg-focus'
  if slug in ['herbwise','alchemy','firstaid']:skill['required_tool']='sg-herb-kit'
  if slug=='survey':skill['required_tool']='sg-survey-kit'
  if slug in ['banner','mirror']:skill['effects']=[{'kind':'guard','power':power},{'kind':'reflect','power':20}]
  if slug=='seal':skill['effects']=[{'kind':'control','power':1},{'kind':'mental_attack','power':14}]
  if slug=='resolve':skill['effects']=[{'kind':'guard','power':power},{'kind':'mental_guard','power':45}]
  if slug=='purge':skill['materials']={'sg-salt':1,'sg-herbs':1}
  if index%8>0 and rank>=3:skill['prerequisite']='sg-'+SKILLS[index-index%8][0]
  skills.append(skill);definitions[ident]={'name':en,'description':den,'background':ben}
  mid='sg-manual-'+slug;mn=zh+'习本';men=en+' Practice Notes';names[mid]=men
  mdesc='纸页记录练习顺序与常见失误，持有习本只是开始，真正掌握需要付出时间。';medesc='Pages record practice steps and common errors. Owning them is a beginning; mastery takes time.'
  manual={'id':mid,'name':mn,'kind':'tool','scale':'person','price':12+rank*5+hours,'power':0,'acquisition':'market',**metadata(mdesc,medesc,f'{place}允许流通的练习抄本，不包含未公开的个人秘密。',f'A circulating practice copy from {p_en}, containing no private personal secrets.',grade)}
  items.append(manual);definitions[mid]={'name':men,'description':medesc,'background':manual['localizations']['en-US']['background']}
 recipes=[]
 specs=[('tonic','alchemy',1,1,{'herbs':2,'cloth':1},'tonic',2),('salve','herbwise',1,1,{'herbs':2,'salt':1},'salve',2),('draught','alchemy',1,2,{'herbs':2,'spirit':1},'rest-draught',2),('greater-tonic','alchemy',2,3,{'herbs':4,'spirit':2,'cloth':2},'greater-tonic',2),('rescue','alchemy',3,4,{'herbs':5,'salt':2,'spirit':2},'rescue-dose',1),('ration','herbwise',1,1,{'grain':3,'salt':1},'trail-ration',3),('blade','forge',1,3,{'iron':2,'wood':1},'travel-blade',1),('spear','forge',1,4,{'iron':3,'wood':2},'ash-spear',1),('dagger','forge',1,2,{'iron':2,'wood':1},'dagger',1),('mail','forge',2,8,{'iron':10,'leather-strip':2},'mail',1),('leather','leatherwork',1,4,{'leather-strip':5,'cloth':2},'leather',1),('rune-coat','runes',2,8,{'cloth':8,'silver-wire':5,'crystal':4},'rune-coat',1),('root-mail','leatherwork',2,8,{'wood':6,'resin':5,'leather-strip':5},'root-mail',1),('staff','runes',1,4,{'wood':3,'silver-wire':2,'crystal':2},'focus-staff',1),('rune-sabre','masterforge',1,10,{'iron':15,'silver-wire':6,'crystal':4},'rune-sabre',1),('glass-lance','glasswork',2,12,{'glass':12,'iron':10,'resin':4},'glass-lance',1),('forgeplate','masterforge',2,16,{'iron':30,'leather-strip':8,'silver-wire':2},'forgeplate',1),('beacon-edge','beaconwork',2,20,{'crystal':25,'silver-wire':15,'iron':20},'beacon-edge',1),('fire','alchemy',1,2,{'charcoal':3,'resin':2,'spirit':1},'fire-flask',2),('arrows','forge',1,1,{'wood':1,'iron':1},'arrow',10)]
 itemmap={i['id']:i for i in items}
 for slug,sk,mastery,hours,mats,output,count in specs:
  ident='sg-recipe-'+slug;item=itemmap['sg-'+output];en=names['sg-'+output]+' Making';names[ident]=en
  rec={'id':ident,'name':item['name']+'制法','skill_id':'sg-'+sk,'skill_rank':mastery,'hours':hours,'materials':{'sg-'+k:v for k,v in mats.items()},'output_id':'sg-'+output,'output_count':count,**metadata('按材料性状分步处理，失败或改制仍需记录实际去向。','Work in stages appropriate to the materials, recording what is actually used or changed.','诸境工匠愿意传授的实用配方；熟练程度与材料共同约束成品。','A practical shared method constrained by mastery and actual materials.','green' if mastery<2 else 'blue')}
  recipes.append(rec);definitions[ident]={'name':en,'description':rec['localizations']['en-US']['description'],'background':rec['localizations']['en-US']['background']}
 starter=['sg-blade','sg-parley'];starteritems={'sg-travel-blade':1,'sg-padded':1,'sg-bread':3,'sg-tonic':1,'sg-manual-blade':1,'sg-manual-parley':1,'sg-journal':1}
 rules={'family':'fantasy','currency':'银铢','skills':skills,'items':items,'recipes':recipes,'opponents':[],'npcs':[],'markets':[],'starting_skills':starter,'starting_items':starteritems,'starting_currency':24,'repair_material_id':'sg-iron'}
 foundations=['blade','spark','trail','firstaid','parley']
 rules['progression']={'max_rank':30,'experience_per_hour':12,'currency_per_hour':2,'stages':[{'id':f'sg-stage-{n}','name':zh,'english_name':en,'min_rank':rank,'practice_skill':'sg-'+foundations[0],'practice_alternatives':['sg-'+x for x in foundations[1:]],'practice_mastery':mastery} for n,(zh,en,rank,mastery) in enumerate([('行路人','Wayfarer',1,1),('熟练者','Practiced',5,2),('守望者','Warden',10,3),('承业者','Adept',16,4),('立誓者','Oathbearer',23,5)],1)]}
 return rules,names,definitions
