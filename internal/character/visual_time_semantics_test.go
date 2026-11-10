package character

import "testing"

func TestVisualObservationPreservesNonCalendarTime(t *testing.T) {
	for _, label := range []string{"第3日午后", "山门重开当日", "1991年9月1日"} {
		c, f := visualFixture()
		c.WorldTime, c.Scene.WorldTime = label, label
		out, err := Project(c, f)
		if err != nil {
			t.Fatal(err)
		}
		if out.Knowledge[0].Visual.Year != ExplicitVisualYear(label) || out.Knowledge[0].WorldTime != label {
			t.Fatal("observation invented a calendar or lost source time")
		}
		for _, e := range out.Exchanges {
			if e.WorldTime != label {
				t.Fatal("source time lost")
			}
		}
	}
}

func TestVisualObservationRefusesUnboundTime(t *testing.T) {
	for _, change := range []string{"empty", "mismatched context", "contact time", "recognition time"} {
		t.Run(change, func(t *testing.T) {
			c, f := visualFixture()
			c.WorldTime, c.Scene.WorldTime = "第3日午后", "第3日午后"
			out, err := Project(c, f)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "empty":
				c.WorldTime, c.Scene.WorldTime = "", ""
			case "mismatched context":
				c.WorldTime = "第4日午后"
			case "contact time":
				out.Exchanges[0].WorldTime = "第4日午后"
			case "recognition time":
				out.Exchanges[1].WorldTime = "第4日午后"
			}
			if _, err := projectVisualObservation(c, c.Scene, f.Knowledge[0], out.Exchanges); err == nil {
				t.Fatal("unbound time accepted")
			}
		})
	}
}
