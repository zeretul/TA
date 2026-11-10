package character

import "testing"

func TestVisualRoleRequiresCompleteAffirmativeCurrentClause(t *testing.T) {
	negatives := []string{
		"我想成为天文学教授。", "我曾是天文学教授。", "我将成为天文学教授。",
		"我的朋友是天文学教授。", "天文学教授的助手由我联络。", "我是天文学教授的助手。",
		"天文学教授已经离职了。", "天文学教授，只是以前。", "天文学教授。那是过去。",
		"我是天文学教授，但已经离职。", "她说我是天文学教授。", "‘天文学教授’是别人叫的。",
	}
	for _, tail := range negatives {
		t.Run(tail, func(t *testing.T) {
			ctx, facts := visualFixture()
			facts.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔，" + tail
			out, err := Project(ctx, facts)
			if err != nil {
				return
			} // Conservatively refusing the whole observation is safe.
			v := out.Knowledge[0].Visual
			p := ctx.VisualContext.Subjects["sinistra"].Profile
			if VisualRecognitionSupports(out.Exchanges[1], p, "protagonist") || v.Role != "" || v.ProfileRevision != p.Revision {
				t.Fatalf("non-current role mention became role evidence: %+v", v)
			}
		})
	}
}

func TestVisualRoleAffirmativeClausesAndAuthoredCommaRemainSupported(t *testing.T) {
	for _, tc := range []struct{ role, text string }{
		{"天文学教授", "我是奥罗拉·辛尼斯塔，天文学教授。"},
		{"天文学教授", "我叫奥罗拉·辛尼斯塔，我是天文学教授。"},
		{"天文学教授", "我是奥罗拉·辛尼斯塔。 我是天文学教授！"},
		{"天文学教授，负责天文课程", "我是奥罗拉·辛尼斯塔，天文学教授，负责天文课程。"},
		{"天文学教授，负责天文课程。", "我叫奥罗拉·辛尼斯塔，我是天文学教授，负责天文课程。"},
	} {
		ctx, facts := visualFixture()
		subject := ctx.VisualContext.Subjects["sinistra"]
		subject.Profile.Role = tc.role
		ctx.VisualContext.Subjects["sinistra"] = subject
		facts.Exchanges[1].Text = tc.text
		out, err := Project(ctx, facts)
		if err != nil {
			t.Fatal(err)
		}
		if !VisualRecognitionSupports(out.Exchanges[1], subject.Profile, "protagonist") || out.Knowledge[0].Visual.Role != tc.role {
			t.Fatalf("affirmative role rejected: %q", tc.text)
		}
	}
}
