package history

func initialState(id, name string) State {
	return State{ID: id, Name: name, ScenarioID: "ming-1627-slice", Year: 1627, Quarter: 3,
		Treasury: 1000000, PrivyPurse: 1500000, Grain: 630000,
		Regions: []Region{
			{ID: "beizhili", Name: "北直隶", Heart: 55, TaxBase: 450000},
			{ID: "liaodong", Name: "辽东", Heart: 35, TaxBase: 80000, Training: 25},
			{ID: "shaanxi", Name: "陕西", Heart: 40, TaxBase: 250000, Disaster: 22, Refugees: 5000},
			{ID: "shanxi", Name: "山西", Heart: 45, TaxBase: 220000, Refugees: 500},
		},
		Ministers: []Minister{
			{ID: "bi-ziyan", Name: "毕自严", Office: "户部侍郎（候任）", Profile: "财政奇才，精于核算，勤勉务实。", Domain: "finance", Capacity: 100000, Knowledge: []string{"finance", "regions"}},
			{ID: "fan-jingwen", Name: "范景文", Office: "工部侍郎", Profile: "廉直忠勤，实务干才，擅长工程与钱粮核算。", Domain: "construction", Capacity: 50000, Knowledge: []string{"regions", "construction"}},
			{ID: "sun-chengzong", Name: "孙承宗", Office: "在野（辽东老帅）", Profile: "稳重持国，车营大家，善于练兵守御。此试作中奉召参议军务。", Domain: "military", Capacity: 100000, Knowledge: []string{"military", "regions"}},
			{ID: "wang-chengen", Name: "王承恩", Office: "司礼监秉笔太监", Profile: "忠谨沉默，皇帝亲信，掌握内廷事务。", Domain: "court", Capacity: 20000, Knowledge: []string{"privy", "regions"}},
		}, Projects: []Project{}, Edicts: []Edict{}, Chats: []ChatTurn{}, Reports: []QuarterReport{}, Checkpoints: []Checkpoint{}, Inspections: []Inspection{},
	}
}
