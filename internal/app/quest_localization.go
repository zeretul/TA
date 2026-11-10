package app

import (
	"fmt"
	"strings"
	"ta/internal/interactive"
	"ta/internal/quest"
	"ta/internal/storypack"
)

func localizeQuestViews(state *interactive.QuestState, locale string, metas ...interactive.StoryMeta) {
	if !strings.HasPrefix(locale, "en") {
		return
	}
	for i := range state.Available {
		view := &state.Available[i]
		if d, ok := questDefinitionForView(view.TaskID, metas); ok {
			if len(metas) > 0 {
				d = interactive.LocalizedStoryQuest(metas[0], d, locale)
			} else {
				d = quest.LocalizedDefinition(d, locale)
			}
			view.Title, view.Brief, view.Choices = d.Title, d.Brief, quest.VisibleChoices(d, nil)
		}
	}
	for i := range state.Tasks {
		view := &state.Tasks[i]
		if d, ok := questDefinitionForView(view.TaskID, metas); ok {
			if len(metas) > 0 {
				d = interactive.LocalizedStoryQuest(metas[0], d, locale)
			} else {
				d = quest.LocalizedDefinition(d, locale)
			}
			view.Title, view.Brief, view.Choices = d.Title, d.Brief, quest.VisibleChoices(d, view.Resolutions)
			for j := range view.Objectives {
				for _, objective := range d.Objectives {
					if objective.ID == view.Objectives[j].ID {
						view.Objectives[j].Description = objective.Description
						break
					}
				}
			}
			view.Consequences = nil
			for _, c := range d.Choices {
				r, ok := view.Resolutions[c.ObjectiveID]
				if !ok || r.ChoiceID != c.ID {
					continue
				}
				immediate, echo := c.Success, c.SuccessEcho
				if r.Outcome == "failure" {
					immediate, echo = c.Failure, c.FailureEcho
				}
				view.Consequences = append(view.Consequences, immediate)
				if r.EchoResolved && (!r.EchoEncounterRequired || c.Reaction(r.Outcome) == nil || r.EchoAcknowledgedBy != "") {
					view.Consequences = append(view.Consequences, echo)
				}
			}
			if d.CompletionEcho != nil && view.CompletionEcho != nil && view.CompletionEcho.AcknowledgedBy != "" {
				view.Consequences = append(view.Consequences, d.CompletionEcho.Reaction.Statement)
			}
		}
		if view.DeadlineSeconds > 0 {
			view.DeadlineText = "Expired"
			if view.RemainingSeconds > 0 {
				view.DeadlineText = fmt.Sprintf("%d seconds remaining", view.RemainingSeconds)
			}
		}
	}
}

// Localize authored place/person labels only from the exact bound pack revision.
// Unrecognized free-form scene text and custom characters remain author-owned.
func localizeQuestScene(state *interactive.QuestState, pack storypack.Pack) {
	text := pack.Localizations["en-US"]
	for _, place := range pack.World {
		if state.CurrentLocation == place.Name {
			if name := text.World[place.ID].Name; name != "" {
				state.CurrentLocation = name
			}
			break
		}
	}
	people := make(map[string]string)
	for _, person := range pack.Characters {
		if name := text.Characters[person.SourceKey].Name; name != "" {
			people[person.Name] = name
		}
	}
	for i := range state.Available {
		if name := people[state.Available[i].Giver]; name != "" {
			state.Available[i].Giver = name
		}
	}
	for i := range state.Tasks {
		if name := people[state.Tasks[i].Giver]; name != "" {
			state.Tasks[i].Giver = name
		}
	}
}

func questDefinitionForView(id string, metas []interactive.StoryMeta) (quest.Definition, bool) {
	if len(metas) > 0 {
		return interactive.StoryQuestDefinition(metas[0], id)
	}
	return quest.DefinitionByID(id)
}
