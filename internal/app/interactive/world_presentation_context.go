package interactiveapp

import (
	"encoding/json"

	"ta/internal/interactive"
)

// This bounded fragment only exposes the frozen labels in the active language.
// Five validated labels fit below 2 KiB and spend the existing world-page budget.
func worldPresentationContext(binding *interactive.StoryPackBinding, locale string) string {
	if binding == nil || binding.Gameplay == nil {
		return ""
	}
	terms := binding.Gameplay.Presentation.ForLocale(locale)
	if terms == (interactive.WorldPresentationTerms{}) {
		return ""
	}
	data, err := json.Marshal(terms)
	if err != nil {
		return ""
	}
	return "本局作者命名（combat为交战、exploration为探索、techniques为能力、partner为伴侣称呼、energy为行动能量）：" + string(data) + "。仅用于自然称呼，不改变行动ID、人物关系事实、数值、资源或可用能力；名称本身不能授予法术、传音、远程通讯或新战斗规则。\n"
}
