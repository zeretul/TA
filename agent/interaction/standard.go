package interaction

import agent "ta/agent"

// Standard validates the built-in ask and permission vocabulary.
func Standard() agent.InteractionPolicy { return agent.StandardInteraction() }
