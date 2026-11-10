package character

import "testing"

func TestVisualSingleSourceIntroduction(t *testing.T) {
	c, f := visualFixture()
	f.Exchanges = f.Exchanges[1:]
	f.Knowledge[0].Visual.ContactExchangeIndex = 1
	f.Knowledge[0].Visual.RecognitionExchangeIndex = 1
	out, err := Project(c, f)
	if err != nil {
		t.Fatal(err)
	}
	v := out.Knowledge[0].Visual
	if len(out.Exchanges) != 1 || v.ContactExchangeID != out.Exchanges[0].ID || v.RecognitionExchangeID != out.Exchanges[0].ID || out.Knowledge[0].SourceExchangeID != out.Exchanges[0].ID || v.Role != "天文学教授" {
		t.Fatalf("single source relation lost: %+v", out)
	}
}

func TestVisualSingleSourceRetainsProvenanceGuards(t *testing.T) {
	tests := map[string]func(*ProjectionContext, *KnowledgeDraft, *Exchange){
		"third party":    func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.CharacterID = "impostor" },
		"wrong observer": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.AddressedTo = "other" },
		"wrong holder":   func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { d.CharacterID = "other" },
		"wrong subject":  func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { d.Visual.SubjectID = "other" },
		"alias prefix": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) {
			e.Text = "我是奥罗拉·辛尼斯塔的助手，天文学教授。"
		},
		"denial": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) {
			e.Text = "我不是奥罗拉·辛尼斯塔，天文学教授。"
		},
		"uncertain": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) {
			e.Text = "我是奥罗拉·辛尼斯塔，可能是天文学教授。"
		},
		"private":           func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.Channel = ChannelPrivate },
		"letter":            func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.Channel = Channel("letter") },
		"observer audience": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.Audience = []string{"sinistra"} },
		"subject audience":  func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.Audience = []string{"protagonist"} },
		"story":             func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.StoryID = "other" },
		"branch":            func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.BranchID = "other" },
		"source":            func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.SourceEventID = "other" },
		"turn":              func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.TurnID = "other" },
		"time":              func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.WorldTime = "1992年9月1日" },
		"scene":             func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { e.SceneNode = "other" },
		"commit missing":    func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { c.CommitID = "" },
		"appearance": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) {
			s := c.VisualContext.Subjects["sinistra"]
			s.Appearance = "看不清"
			c.VisualContext.Subjects["sinistra"] = s
		},
		"form": func(c *ProjectionContext, d *KnowledgeDraft, e *Exchange) { d.Visual.Form = "猫" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c, f := visualFixture()
			out, err := Project(c, f)
			if err != nil {
				t.Fatal(err)
			}
			d := f.Knowledge[0]
			d.Visual.ContactExchangeIndex = 1
			d.Visual.RecognitionExchangeIndex = 1
			e := out.Exchanges[1]
			mutate(&c, &d, &e)
			if _, err := projectVisualObservation(c, c.Scene, d, []Exchange{e}); err == nil {
				t.Fatal("invalid single source accepted")
			}
		})
	}
}
