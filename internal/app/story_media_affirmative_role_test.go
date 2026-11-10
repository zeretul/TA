package app

import "testing"

func TestStoryMediaRolePhraseRequiresSpeakerCurrentAffirmation(t *testing.T) {
	const role = "天文学教授，关心学生的课程与私人生活。"
	for _, tail := range []string{
		"我想成为天文学教授。", "我曾是天文学教授。", "我将成为天文学教授。",
		"我的朋友是天文学教授。", "天文学教授的助手由我联络。", "我是天文学教授的助手。",
		"天文学教授，只是以前。", "天文学教授。那是过去。", "她说我是天文学教授。",
		"我是天文学教授，但已经离职。", "天文学教授，关心学生的课程与私人生活。",
	} {
		t.Run(tail, func(t *testing.T) {
			a, c, _ := storyMediaGateFixture(t)
			a.Rule.Role, a.Rule.RoleRecognition = role, "天文学教授"
			s := rolePhraseStoryMediaState(t, role, "我是奥罗拉·辛尼斯塔，"+tail)
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("mentioned/qualified title authorized portrait")
			}
		})
	}
}

func TestStoryMediaRolePhraseAcceptsOnlyCompleteSelfAffirmation(t *testing.T) {
	for _, line := range []string{"我是奥罗拉·辛尼斯塔，天文学教授。", "我叫奥罗拉·辛尼斯塔，我是天文学教授。"} {
		a, c, _ := storyMediaGateFixture(t)
		a.Rule.Role, a.Rule.RoleRecognition = "天文学教授，关心学生的课程与私人生活。", "天文学教授"
		s := rolePhraseStoryMediaState(t, a.Rule.Role, line)
		if !storyAssetAllowed(a, c, s, "sinistra") {
			t.Fatalf("affirmative reviewed title rejected: %q", line)
		}
	}
	a, c, s := storyMediaGateFixture(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("legacy HP affirmative source rejected")
	}
}
