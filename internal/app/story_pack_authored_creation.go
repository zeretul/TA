package app

import (
	"encoding/json"
	"strings"

	"ta/internal/storypack"
)

// The preview and formal creation use the same existing persona overlay. The
// remaining authored values stay opaque facts, including unknown nested keys.
func identityAuthoredPreview(identity storypack.Identity) (storypack.Identity, error) {
	if identity.InitialState == nil || len(identity.InitialState.AuthoredFields) == 0 {
		return identity, nil
	}
	overlay, err := identityStateOverlay(identity)
	if err != nil {
		return identity, err
	}
	overlay[protagonistNameField] = identity.Name
	initial := *identity.InitialState
	initial.AuthoredFields = append([]storypack.CreationAuthoredField(nil), initial.AuthoredFields...)
	for index := range initial.AuthoredFields {
		field := &initial.AuthoredFields[index]
		if value, ok := overlay[field.Name]; ok {
			field.Value = value
		}
	}
	identity.InitialState = &initial
	return identity, nil
}

func creationAuthoredInstructions(pack storypack.Pack, choice storypack.IdentityChoice, identity storypack.Identity) string {
	var instructions strings.Builder
	if identity.InitialState != nil && len(identity.InitialState.AuthoredFields) > 0 {
		if raw, err := json.Marshal(identity.InitialState.AuthoredFields); err == nil {
			instructions.WriteString("\n\n已选底稿的正式初始事实如下。只补身份叙述，不因故事文字额外发放能力、钱物、职位、关系或资格；不要将旧数据的 family 标记解释为另一套规则。\n")
			instructions.Write(raw)
		}
	}
	if pack.Creation == nil || choice.OriginID != "custom" {
		return instructions.String()
	}
	if rule := strings.TrimSpace(pack.Creation.CustomBackgroundRule); rule != "" {
		instructions.WriteString("\n\n自建背景约束：\n" + rule)
	}
	if pack.Creation.CustomLocationPolicy == "authored_only" {
		instructions.WriteString("\n\n当前开局地点只采用以下已声明名称中的一个，location 必须逐字使用其中名称，不能添加房间等后缀。没有明确要求改变起点时沿用上文已选地点。其他出生地或过去经历仍可写入背景，不视为当前落脚地点：\n")
		for _, location := range pack.Creation.Locations {
			if location.ID != "auto" {
				instructions.WriteString("- " + location.Name + "\n")
			}
		}
	}
	return instructions.String()
}
