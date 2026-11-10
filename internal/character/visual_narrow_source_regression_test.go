package character

import "testing"

func TestVisualIdentityRequiresCompleteNameBoundary(t *testing.T) {
	ctx, facts := visualFixture()
	p := ctx.VisualContext.Subjects["sinistra"].Profile
	base := Exchange{CharacterID: p.ID, Speaker: SpeakerCharacter, AddressedTo: "protagonist"}
	for _, line := range []string{
		"我是奥罗拉·辛尼斯塔的助手，天文学教授让我来找你。",
		"我叫奥罗拉·辛尼斯塔的朋友，天文学教授是她。",
		"我是奥罗拉·辛尼斯塔 的代言人，天文学教授让我转告你。",
		"我是奥罗拉·辛尼斯塔？天文学教授？",
	} {
		base.Text = line
		if VisualRecognitionSupports(base, p, "protagonist") {
			t.Fatalf("identity prefix mistaken for identity: %q", line)
		}
	}
	base.Text = "我是奥罗拉·辛尼斯塔，天文学教授。"
	if !VisualRecognitionSupports(base, p, "protagonist") {
		t.Fatal("explicit complete name was rejected")
	}
	facts.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔的助手，天文学教授让我来找你。"
	if _, err := Project(ctx, facts); err == nil {
		t.Fatal("formal projection accepted a representative as the named identity")
	}
}

func TestVisualFormRequiresAffirmativeCompleteLeadingClause(t *testing.T) {
	const form = "人类女性"
	for _, appearance := range []string{"不是人类女性", "看不清是否人类女性", "疑似人类女性", "人类女性雕像", "人类女性？", "人类女性，实际并非人类女性", "人类女性，但看不清，不能确认"} {
		ctx, facts := visualFixture()
		subject := ctx.VisualContext.Subjects["sinistra"]
		subject.Appearance = appearance
		ctx.VisualContext.Subjects["sinistra"] = subject
		if _, err := Project(ctx, facts); err == nil {
			t.Fatalf("formal projection accepted unproven form: %q", appearance)
		}
	}
	for _, appearance := range []string{"人类女性", "人类女性，穿深色教师长袍", "人类女性。深色长袍。"} {
		ctx, facts := visualFixture()
		subject := ctx.VisualContext.Subjects["sinistra"]
		subject.Appearance = appearance
		ctx.VisualContext.Subjects["sinistra"] = subject
		facts.Knowledge[0].Visual.Form = form
		if _, err := Project(ctx, facts); err != nil {
			t.Fatalf("explicit form rejected: %q: %v", appearance, err)
		}
	}
}
