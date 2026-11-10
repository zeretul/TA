package context

import agentcontext "ta/agent/context"

// Assembler is the reusable bounded assembler configured with Text Adventure's prompt
// renderer. The core policy remains in the standalone agent module; only the
// product's localized wrapper text lives here.
type Assembler = agentcontext.Assembler

func NewAssembler(budget Budget) *Assembler {
	return agentcontext.NewAssembler(budget, agentcontext.WithRenderer(taRenderer{}))
}

type taRenderer struct{}

func (taRenderer) RenderLeading(fragment Fragment) string {
	return StandaloneMessage(fragment.Title, renderSourceContent(sourceFromFragment(fragment)), "")
}

func sourceFromFragment(fragment Fragment) Source {
	return Source{
		Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
		Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
		Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
	}
}

func (taRenderer) RenderFinalUser(userRequest string, fragments []Fragment) string {
	sources := make([]Source, 0, len(fragments))
	for _, fragment := range fragments {
		sources = append(sources, Source{
			Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
			Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
			Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
		})
	}
	return PrependFinalUserSources(userRequest, sources)
}
