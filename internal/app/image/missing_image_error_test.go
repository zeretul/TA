package imageapp

import (
	"errors"
	"strings"
	"testing"
)

// The player's report is the only place the reason survives. A picture that
// was generated and then could not be saved, and a model that never called
// the tool, are different failures with different fixes, so they must not
// collapse into one sentence.
func TestMissingImageErrorKeepsTheToolFailure(t *testing.T) {
	result := AgentGenerateResult{
		imageToolCalled: true,
		imageToolError:  &ImageToolError{Detail: `{"status":"error","error":"保存互动图像失败: open ...: permission denied"}`},
	}
	err := result.MissingImageError()
	if err == nil {
		t.Fatal("a failed tool call must not report as a clean success")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the tool's own reason was dropped: %v", err)
	}
}

func TestMissingImageErrorDistinguishesUncalledTool(t *testing.T) {
	result := AgentGenerateResult{}
	if err := result.MissingImageError(); !errors.Is(err, ErrImageToolNotCalled) {
		t.Fatalf("an uncalled tool must report ErrImageToolNotCalled, got %v", err)
	}

	called := AgentGenerateResult{imageToolCalled: true}
	if err := called.MissingImageError(); !errors.Is(err, ErrImageOutputMissing) {
		t.Fatalf("a called tool without output must report ErrImageOutputMissing, got %v", err)
	}
}

// A saved image outranks a stale tool error: the tool may fail while reporting
// a picture that already reached disk, and the caller checks canonical state
// before asking. This pins the precedence the caller depends on.
func TestMissingImageErrorPrefersTheToolFailureOverTheGenericOne(t *testing.T) {
	result := AgentGenerateResult{
		imageToolCalled: true,
		imageToolError:  &ImageToolError{Detail: "保存互动图像失败"},
	}
	if !strings.Contains(result.MissingImageError().Error(), "保存互动图像失败") {
		t.Fatal("the specific failure must win over the generic one")
	}
	uncalled := AgentGenerateResult{}
	if strings.Contains(uncalled.MissingImageError().Error(), "保存互动图像失败") {
		t.Fatal("a run that never called the tool must not borrow the other failure's text")
	}
}
