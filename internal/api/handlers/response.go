package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"ta/internal/i18n"
	"ta/internal/observability"
)

// decodeStrictJSONRequest rejects unknown fields and trailing JSON values at
// mutation boundaries, where silently accepting a caller typo is unsafe.
func decodeStrictJSONRequest(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

// writeJSON 写入 JSON 响应。
func writeJSON(c *app.RequestContext, code int, obj interface{}) {
	c.JSON(code, obj)
}

// writeError 写入错误响应。
func writeError(c *app.RequestContext, code int, msg string) {
	c.JSON(code, map[string]string{"error": msg})
}

func requestLocalizer(c *app.RequestContext) i18n.Localizer {
	return i18n.FromHeader(requestLocaleHeader(c))
}

func requestLocale(c *app.RequestContext) string {
	header := requestLocaleHeader(c)
	if header == "" {
		return ""
	}
	return i18n.FromHeader(header).Locale()
}

// The product's own header name is the one clients send. The two older names are
// still read so a browser tab left open across the rename keeps its language
// instead of silently falling back to the default one.
func requestLocaleHeader(c *app.RequestContext) string {
	for _, name := range []string{"X-TA-Locale", "X-Denova-Locale", "X-Nova-Locale"} {
		if header := strings.TrimSpace(string(c.Request.Header.Peek(name))); header != "" {
			return header
		}
	}
	return ""
}

func writeErrorKey(c *app.RequestContext, code int, key string, args ...any) {
	values := append([]any(nil), args...)
	details := map[string]any{}
	for index := 0; index+1 < len(values); index += 2 {
		name, ok := values[index].(string)
		if !ok {
			continue
		}
		switch name {
		case "detail":
			if value, ok := values[index+1].(string); ok {
				values[index+1] = observability.DiagnosticText(value)
				details["detail"] = values[index+1]
			}
		case "session_id":
			// A refusal can name the battle that is already running, so the
			// window can open it instead of guessing. The identity is not secret:
			// it is the id the same window reads and plays against.
			if value, ok := values[index+1].(string); ok && strings.TrimSpace(value) != "" {
				details["session_id"] = strings.TrimSpace(value)
			}
		}
	}
	writeAgentRuntimeError(c, code, key, requestLocalizer(c).T(key, values...), details)
}

func messageKey(c *app.RequestContext, key string, args ...any) string {
	return requestLocalizer(c).T(key, args...)
}

// requireWorkspace 校验当前 App 是否已绑定 workspace；
// 未绑定时直接写入 409 错误并返回 false，由调用方 return 终止处理。
func (h *Handlers) requireWorkspace(c *app.RequestContext) bool {
	if h.app.HasWorkspace() {
		return true
	}
	writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	return false
}
