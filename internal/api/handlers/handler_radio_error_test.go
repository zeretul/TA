package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	runtimeapp "ta/internal/app"
)

// Both single-person sends and mixed radio groups use this domain refusal.
// The transport code stays machine-readable; the summary must be human text in
// the request's language rather than the untranslated key shown by the UI.
func TestRadioContactRefusalUsesRequestLanguageAndConflictStatus(t *testing.T) {
	for name, write := range map[string]func(*app.RequestContext, error){
		"group":     writeGroupError,
		"character": writeCharacterWindowError,
	} {
		for locale, want := range map[string]string{
			"zh-CN": "当前无法无线电联络：需要实际持有通讯设备，并有已建立且当前可达的联络频道。",
			"en-US": "Radio is unavailable: an owned device and an established, reachable contact channel are required.",
		} {
			t.Run(name+"/"+locale, func(t *testing.T) {
				ctx := app.NewContext(0)
				ctx.Request.Header.Set("X-TA-Locale", locale)
				write(ctx, fmt.Errorf("contact unavailable: %w", runtimeapp.ErrRadioContactUnavailable))
				var response agentRuntimeErrorResponse
				if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
					t.Fatal(err)
				}
				if ctx.Response.StatusCode() != 409 || response.Code != "api.characterWindow.radioUnavailable" || response.Error != want {
					t.Fatalf("unexpected localized refusal: status=%d response=%+v", ctx.Response.StatusCode(), response)
				}
			})
		}
	}
}
