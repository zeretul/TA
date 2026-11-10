package storypack

import "encoding/json"

// Stable delivered world identities are independent of display names.
const builtinPackIDFanren = "fanren-jie"

// HistoricalSanguoPackID remains resolvable for previously bound stories.
const HistoricalSanguoPackID = "sanguo-190-ta07-full-gm-candidate"

// builtinPacks returns the packs compiled into this build. They seed the
// catalog on first run so the delivered version is playable before any user
// pack exists; a same-ID file in the data root overrides them.
//
// 凡人人界 comes from the embedded asset (assets.go) because it carries the
// world's 102-person catalog. Anything devBuiltinPacks adds is a development
// and test world and is absent from a delivered build; see builtin_dev.go.
func builtinPacks() []Pack {
	documents := append([]string{builtinPackJSONFanrenFormal, builtinPackJSONLanchuan, builtinPackJSONXinghai, builtinPackJSONShatteredCrown, builtinRuleCandidate0, builtinRuleCandidate1, builtinRuleCandidate2, builtinRuleCandidate3, builtinRuleCandidate4, builtinRuleCandidate5}, devBuiltinPacks()...)
	packs := make([]Pack, 0, len(documents))
	for _, raw := range documents {
		var pack Pack
		if err := json.Unmarshal([]byte(raw), &pack); err != nil {
			// Builtin documents are compiled in and covered by tests; a parse
			// failure here is a build defect, not a runtime condition.
			panic("storypack: 内置剧本包无法解析: " + err.Error())
		}
		if (pack.Story.Gameplay.StatusTemplate == WorldStatusGeneralActions && pack.Story.Gameplay.Confirmed) || pack.Story.Gameplay.CultivationRules != nil || pack.Story.Gameplay.WorldContent != nil || pack.Story.Gameplay.StatusTemplate == "wuxia-state-v1" || pack.Story.Gameplay.StatusTemplate == "space-state-v1" || pack.Story.Gameplay.StatusTemplate == "fantasy-state-v1" {
			stateID, ruleID := gameplayModuleIDs(pack.ID, pack.Story.Gameplay)
			pack.Story.ModuleRefs.ActorStateID = stateID
			pack.Story.ModuleRefs.RuleSystemID = ruleID
		}
		pack.Builtin = true
		packs = append(packs, pack)
	}
	for index := range packs {
		switch packs[index].ID {
		case "doupo-continent-ta07-full-gm-candidate":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinRuleCandidateEnglish0)}
		case "harry-potter-1991-student-ta07-full-gm-candidate":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinRuleCandidateEnglish1)}
		case "harry-potter-1991-ta07-full-gm-candidate":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinRuleCandidateEnglish2)}
		case "lord-of-mysteries-ta07-full-gm-candidate":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinRuleCandidateEnglish3)}
		case "zichuan-xichuan-ta07-full-gm-candidate":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinRuleCandidateEnglish4)}
		case builtinPackIDFanren:
			packs[index].Localizations = map[string]Localization{
				"en-US": builtinFanrenEnglish(),
			}
		case "lanchuan-qunxia":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinPackLocalizationLanchuanEN)}
		case "sui-guan-realms":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinPackLocalizationShatteredCrownEN)}
		case "xinghai-689":
			packs[index].Localizations = map[string]Localization{"en-US": builtinEnglish(builtinPackLocalizationXinghaiEN)}
		}
	}
	return packs
}

// builtinFanrenEnglish parses the delivered English display text for the
// builtin world. It is mounted as one entry of that pack's Localizations, so
// every page that asks for English reads the same prose while the ids, numbers
// and rule values it renders stay the Chinese pack's own.
//
// The asset is compiled in, so a parse failure is a build defect and panics the
// same way a broken pack does.
func builtinFanrenEnglish() Localization {
	return builtinEnglish(builtinPackLocalizationFanrenEN)
}

func builtinEnglish(raw string) Localization {
	var localization Localization
	if err := json.Unmarshal([]byte(raw), &localization); err != nil {
		panic("storypack: 内置剧本包英文文案无法解析: " + err.Error())
	}
	return localization
}
