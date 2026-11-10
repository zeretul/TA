"""Human-reviewed corrections on independent model prose, keyed by choice ID."""
import json,re
from pathlib import Path
OUT=Path(__file__).resolve().parents[4]/'work/astra-completion-20261102/content'
data=json.loads((OUT/'quest-prose-reviewed.json').read_text(encoding='utf8'))
fixes={
'lanchuan-a01.c2':dict(title='向宋青领取托给老父的护膝软布',description='在清河城接下宋青缝好的护膝软布，听她交代想托给老父的话。',success='宋青将针脚细密的软布包好交给你，叮嘱别让它沾了江水。东西尚待送到潮信渡，老父会如何回应，还没有人知道。',success_echo='渡口尚未收到这包软布。宋青托付的话仍在耳边，你可以将它亲手交给宋老橹，听听他的回答。'),
'lanchuan-a04.c1':dict(success_echo='谢铁臂离开擂台，镇海武馆的弟子仍在争论方才几招的得失。管山海邀你闲时再来，话头落在码头最近的风声上。'),
'lanchuan-a04.c3':dict(success_echo='郑铁牛卸下护腕，兴奋地复盘肘底那处破绽。管山海在旁听着，不时指点两句，馆里重新响起练拳的呼喝。'),
'lanchuan-a05.c1':dict(success='抵达清河城后，你在瓷行逐项核对货单，说明途中查验的情形。掌柜验过底釉和封条，收下货物，按约结清脚力钱。'),
'lanchuan-a05.c2':dict(success='船已靠岸清河城，你与瓷行掌柜核对水运货单和封条。暗镖交割无误，掌柜签下回执，按约结清脚力钱。'),
'lanchuan-a05.c3':dict(success='你在清河城与晏小五对清真镖和疑车的交接记录。瓷行验货无误，将交割回执和约定脚力钱一并交来。'),
'lanchuan-a12.c1':dict(success='十日里，你逐页誊清经文，反复校正错漏。纪闻墨验过字迹与卷次，结清工钱，把约好的拳谱誊本一并交来。'),
'lanchuan-a20.c2':dict(success='燕寒炉将剑坯郑重相托，叮嘱你仔细裹好残锋。你照约将它送往烟波庄；容九皋认出旧痕，默然伸手接住。',success_echo='烟波庄传来回信，说剑坯已经收妥。燕寒炉读了两遍，把信压在案角，许久没有举锤。'),
'lanchuan-a20.c3':dict(success_echo='听雪庐的灯火比往日亮得久。柳沉舟把剑坯放在案上，对旧事仍说得很少；燕寒炉得知东西送到，也只是默默点头。'),
'xinghai-a01.c1':dict(title='到船坞组装弹链抵清泊位欠费',success='弹链拼装齐整，黑星清点后将约定的480伊纳直接结给港务处。这笔泊位欠费有了正式回执。',success_echo='船坞工头看过交付回执，记下了你的手艺。外环照常执行维护，往返时仍须留意气闸广播。'),
'xinghai-a01.c2':dict(success='铜芯交到港务回收处，蒙托核过重量，用回收款抵清480伊纳欠费，给出结清回执。'),
'xinghai-a01.c3':dict(success='伪造记录通过了眼前的核验，催缴终端暂时转绿。真实欠款仍是480伊纳；例行维护的气闸广播没有因此停下。'),
'xinghai-a09.c1':dict(title='持枪击退贩子带席尔瓦离开',success='枪声逼得两名贩子丢下控制器退入巷口。你带席尔瓦离开污水坑，她攥紧衣襟，跟你沿通往三号船坞的路撤去。'),
'xinghai-a10.c3':dict(success_echo='血狐已在安全处接受救治，她记得走廊里那团救急的冷雾。远处的通讯频段上，仍有人追问爆炸与样本的去向。'),
'xinghai-a12.c2':dict(title='制服下水道死士夺取催化黑匣',success='你压住对方的武器，在自毁装置启动前夺下黑匣。里面保存的催化记录被送往检验，调查人员据此核发了约定酬劳。'),
'xinghai-a13.c3':dict(success_echo='弗丁重新拨响琴弦，试着确认自己能否控制念力的起伏。晨星的声音仍藏在他的识海里，只偶尔随音符轻轻回应。'),
'xinghai-a16.c1':dict(success_echo='试爆记录铺满工作台，沃里克一边核对裂口深度，一边争论下一次测试该选哪块靶材。'),
'xinghai-a16.c2':dict(success_echo='伏尔加兄弟轮流查看切削完成的锥形罩，讨论如何把这次做法用于下一批工程部件。'),
'xinghai-a16.c3':dict(success_echo='爆坑附近的仪器陆续传回读数。沃里克把成功的那组参数圈了起来，叫弟弟们别再乱动调校旋钮。'),
'xinghai-a17.c3':dict(success_echo='净水主干线的压力恢复正常。莉娜让人来问备用泵还能撑多久，工兵们已开始讨论下一轮检修。'),
'xinghai-a18.c2':dict(title='操纵机炮逼退海盗灭口快艇',success='炮火封住了快艇的接近路线，敌艇带着受损的外壳退出碎石带。你们趁机保住逃生舱里的账目与债券。',success_echo='迦兰报告追来的艇已退到雷达边缘，仍不能断言对方会就此放弃。她催促你收好账目，准备离开碎石带。'),
'xinghai-a19.c1':dict(success_echo='使者收存了拓扑图的副本，附近技师围着几处特殊节点低声议论。古齿轮可以证明这次考校，如何再见神域来客仍要另寻途径。'),
'xinghai-a20.c1':dict(success_echo='副官翻过考核记录，指向先锋舰队的接洽处。校场仍在点名，是否继续这条从军道路，下一步仍由你决定。'),
'xinghai-a20.c2':dict(success_echo='副炮完成了低负荷自检，监考技师记下故障原因。接洽军官留下报到地点，等你进一步说明打算。'),
'xinghai-a20.c3':dict(success_echo='军令状已由接洽处收存，军官说明后续报到与核验安排。远征尚未启程，码头上仍有许多准备工作。'),
}
for key,fields in fixes.items():data[key].update(fields)
for key,row in data.items():
    for field in ['title','description','success','failure','success_echo','failure_echo']:
        assert row.get(field), (key,field)
        row[field]=row[field].replace('凌空凌碎','凌空碎裂').replace('残骸残骸','残骸').replace('随随身','随身')
        row[field]=re.sub(r'（[^）]*(?:独立耗材|不同于I)[^）]*）','',row[field])
    row['review_status']='reviewed-candidate-reconcile-current-rule-results'
    row['review_note']=row.get('review_note','')
    if key in fixes:row['review_note']+=' 人工复核：已修正分支越权、延迟重复奖励、错性别、未提交死亡或时限差异。'
(OUT/'quest-prose-reviewed.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
print(len(data),'reviewed candidates;',len(fixes),'manual branch corrections')
