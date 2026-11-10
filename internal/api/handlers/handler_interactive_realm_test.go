package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"ta/internal/interactive"
)

func TestRealmApproachRequestCarriesOnlyAuthoredIdentity(t *testing.T) {
	for _, action := range []string{"survey", "prepare", "approach", "resolve"} {
		t.Run(action, func(t *testing.T) {
			var body realmActionBody
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"command_id":"command-1","branch":"main","instance_id":"instance-1","action":%q,"approach_id":"route-careful","locale":"zh-CN","duration_seconds":0,"capabilities":["anything"],"rewards":[{"ref_id":"coin","quantity":99999}]}`, action)), &body); err != nil {
				t.Fatal(err)
			}
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-TA-Locale", "en-US")
			request := body.request("story-1", requestLocale(ctx))
			if request.StoryID != "story-1" || request.BranchID != "main" || request.InstanceID != "instance-1" || request.Action != action || request.ApproachID != "route-careful" || request.CommandID != "command-1" || request.Locale != "en-US" {
				t.Fatalf("authored choice lost at API boundary: %+v", request)
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var values map[string]any
			if err := json.Unmarshal(encoded, &values); err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"duration_seconds", "capabilities", "rewards", "locale"} {
				if _, exists := values[forbidden]; exists {
					t.Fatalf("client controlled rule result %q", forbidden)
				}
			}
		})
	}
}

func TestRealmApproachConflictIsAnHTTPConflict(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("X-TA-Locale", "en-US")
	writeRealmError(ctx, fmt.Errorf("changed approach: %w", interactive.ErrRealmCommandConflict))
	var body agentRuntimeErrorResponse
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if ctx.Response.StatusCode() != 409 || body.Code != "api.realm.commandConflict" || body.Error == "" {
		t.Fatalf("wrong command conflict response: %d %+v", ctx.Response.StatusCode(), body)
	}
}
