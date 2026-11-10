package i18n

func init() {
	zh := map[string]string{
		"api.history.modelOutputBudget":  "模型的单次输出额度已耗尽，本季尚未保存。请在游戏模型设置中降低思考档位或提高输出额度后重试。",
		"api.history.busy":               "本局正在处理上一项操作，请等待完成或先取消。",
		"api.history.conflict":           "本局已经有新变化，请刷新后再操作。原有记录没有被覆盖。",
		"api.history.notFound":           "找不到这份历史推演存档。",
		"api.history.invalidRequest":     "这项操作的信息不完整或超出当前支持范围，请检查后重试。",
		"api.history.modelFailed":        "本次 AI 请求未完成，尚未保存本次操作。请检查游戏模型设置后重试。",
		"api.history.invalidModelOutput": "AI 返回的内容无法用于本次操作，尚未保存本次操作，可以重试。",
		"api.history.cancelled":          "操作已取消。请刷新查看最新存档。",
		"api.history.wrongProject":       "请从历史推演入口打开独立存档。",
		"api.history.commandConflict":    "这个操作编号已用于另一项请求，请刷新后重新操作。",
		"api.history.unavailable":        "暂时无法读取或保存历史推演，请重试。已有存档仍被保留。",
	}
	en := map[string]string{
		"api.history.modelOutputBudget":  "The model exhausted its response budget. This season was not saved. Lower the thinking level or raise the output budget in game model settings, then retry.",
		"api.history.busy":               "This campaign is processing another action. Wait for it to finish or cancel it first.",
		"api.history.conflict":           "This campaign has changed. Refresh before trying again. Existing records were not overwritten.",
		"api.history.notFound":           "This historical campaign could not be found.",
		"api.history.invalidRequest":     "This action is missing information or exceeds the supported scope. Check it and try again.",
		"api.history.modelFailed":        "The AI request did not finish. This action was not saved. Check the game model settings and retry.",
		"api.history.invalidModelOutput": "The AI response could not be used. This action was not saved; you can retry.",
		"api.history.cancelled":          "The action was cancelled. Refresh to see the latest saved state.",
		"api.history.wrongProject":       "Open an independent campaign from Historical Simulation.",
		"api.history.commandConflict":    "This action ID was already used for a different request. Refresh and try again.",
		"api.history.unavailable":        "The historical campaign could not be read or saved. Please retry. Existing saves are preserved.",
	}
	for key, value := range zh {
		catalogZH[key] = value
	}
	for key, value := range en {
		catalogEN[key] = value
	}
}
