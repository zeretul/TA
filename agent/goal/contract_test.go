package goal_test

import (
	"testing"

	agent "ta/agent"
	"ta/agent/goal"
	"ta/agent/goal/goaltest"
)

func TestStandardManagerContract(t *testing.T) {
	goaltest.RunStandardManagerContract(t, func(testing.TB) agent.GoalManager {
		return goal.Standard()
	})
}
