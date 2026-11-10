package permission_test

import (
	"testing"

	agent "ta/agent"
	"ta/agent/permission"
	"ta/agent/permission/permissiontest"
)

func TestCodingPolicyContract(t *testing.T) {
	permissiontest.RunPolicyContract(t, func(testing.TB) agent.PermissionPolicy {
		policy, err := permission.CodingWithRules(permission.MemoryRules())
		if err != nil {
			t.Fatal(err)
		}
		return policy
	})
}
