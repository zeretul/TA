package agentchat

import (
	"context"
	"fmt"

	agent "ta/agent"

	chatagent "ta/internal/agents/chat"
	agentdelegation "ta/internal/agents/delegation"
	agentexecution "ta/internal/agents/execution"
	agentrun "ta/internal/agents/run"
	appagentruntime "ta/internal/app/agentruntime"
	conversationapp "ta/internal/app/conversation"
)

func (service *Service) PrepareChildDefinition(
	ctx context.Context,
	binding agentrun.RuntimeBinding,
	child string,
	parentRequest ...chatagent.ChatRequest,
) (agent.Definition, error) {
	scope, err := service.ResolveBinding(Binding{
		ProjectID: binding.ProjectID, Workspace: binding.Workspace, SessionID: binding.SessionID,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	project, err := service.projectRuntime(ctx, scope.ProjectID)
	if err != nil {
		return agent.Definition{}, err
	}
	sess, _, err := getOrCreateConversation(project, scope)
	if err != nil {
		return agent.Definition{}, err
	}
	request := chatagent.ChatRequest{}
	if len(parentRequest) > 0 {
		request = parentRequest[0]
	}
	runtime, _, err := conversationapp.Prepare(ctx, project.conversation(sess), request)
	if err != nil {
		return agent.Definition{}, err
	}
	if runtime.ProjectID != scope.ProjectID || runtime.Session == nil || runtime.Session.ID != scope.SessionID {
		return agent.Definition{}, fmt.Errorf("%w: delegated AgentChat parent changed", agentexecution.ErrCyclePreparationUnavailable)
	}
	agentHost, err := service.host.ProjectAgentHostCapabilities(ctx, runtime.ProjectType, &runtime.Config, runtime.AgentKind)
	if err != nil {
		return agent.Definition{}, err
	}
	if err := service.applyToolScope(ctx, scope, sess, TurnPolicy{}, &agentHost); err != nil {
		return agent.Definition{}, fmt.Errorf("scope AgentChat host capabilities: %w", err)
	}
	built, err := appagentruntime.BuildConversationAgent(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
		agentHost,
	)
	if err != nil {
		return agent.Definition{}, err
	}
	return agentdelegation.ChildDefinition(built.Definition, child)
}
