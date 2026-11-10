package tools

import (
	"fmt"
	"strings"

	agenttools "ta/agent/tools"

	workspacechange "ta/internal/workspace/change"
)

// newAgentCommandRunner binds the reusable Process implementation to Text Adventure's
// workspace mutation coordinator, so shell effects cannot race editor saves,
// reviewed changes, undo, or rewind operations.
func newAgentCommandRunner(
	workspace *agenttools.LocalWorkspace,
	shell agenttools.ShellKind,
	executable string,
	environment []string,
	projectStoreRoot string,
) (agenttools.CommandRunner, error) {
	if workspace == nil || workspace.Root() == "" {
		return nil, fmt.Errorf("shell workspace is required")
	}
	var changes *workspacechange.Service
	var err error
	if strings.TrimSpace(projectStoreRoot) != "" {
		changes, err = workspacechange.ForWorkspaceAt(workspace.Root(), projectStoreRoot)
	} else {
		changes, err = workspacechange.ForWorkspace(workspace.Root())
	}
	if err != nil {
		return nil, fmt.Errorf("coordinate shell workspace: %w", err)
	}
	return agenttools.NewLocalCommandRunner(agenttools.CommandRunnerOptions{
		Workspace:       workspace,
		Shell:           shell,
		Executable:      executable,
		BaseEnvironment: append([]string(nil), environment...),
		Guard:           changes.WithExclusiveWorkspace,
	})
}
