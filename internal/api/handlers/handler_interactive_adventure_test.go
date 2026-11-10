package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestAdventureActionRequiresExplicitBranchBeforeApplicationCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		branch any
		status int
		code   string
	}{
		{"missing", nil, 400, "api.common.invalidRequest"},
		{"empty", "", 400, "api.common.invalidRequest"},
		{"whitespace", " \t ", 400, "api.common.invalidRequest"},
		{"explicit", "main", 409, "api.adventure.rejected"},
		{"trimmed explicit", " main ", 409, "api.adventure.rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.SetMethod("POST")
			request.Request.Header.Set("Content-Type", "application/json")
			request.Request.Header.Set("X-TA-Locale", "en-US")
			body := map[string]any{"command_id": "one-round", "expected_head": "head", "action": map[string]string{"action": "battle_round", "choice": "attack"}}
			if tc.branch != nil {
				body["branch"] = tc.branch
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			request.Request.SetBody(encoded)
			// A nil App rejects valid explicit branches as no workspace. The 400
			// cases must stop at request validation before attempting a model/commit.
			(&Handlers{}).HandleInteractiveAdventureAction(context.Background(), request)
			var response agentRuntimeErrorResponse
			if err := json.Unmarshal(request.Response.Body(), &response); err != nil {
				t.Fatal(err)
			}
			if request.Response.StatusCode() != tc.status || response.Code != tc.code || response.Error == "" {
				t.Fatalf("branch validation: status=%d body=%s", request.Response.StatusCode(), request.Response.Body())
			}
		})
	}
}
