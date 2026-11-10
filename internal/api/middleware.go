package api

import (
	"context"
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	runtimeapp "ta/internal/app"
	"ta/internal/i18n"
	"ta/internal/observability"
)

// corsMiddleware 处理 CORS 跨域请求。
func corsMiddleware(ctx context.Context, c *app.RequestContext) {
	origin := string(c.Request.Header.Peek("Origin"))
	allowedOrigins := []string{
		"http://localhost:5173",
		"http://localhost:3000",
		"http://127.0.0.1:5173",
		"http://127.0.0.1:3000",
	}

	allowed := false
	for _, o := range allowedOrigins {
		if strings.EqualFold(origin, o) {
			allowed = true
			break
		}
	}
	if allowed {
		c.Response.Header.Set("Access-Control-Allow-Origin", origin)
	}
	c.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, PUT, OPTIONS")
	c.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, X-TA-Locale, X-Denova-Locale, X-Nova-Locale, Authorization")
	c.Response.Header.Set("Access-Control-Expose-Headers", observability.RequestIDHeader+", X-TA-Auth, X-Denova-Auth")

	if string(c.Request.Method()) == "OPTIONS" {
		c.AbortWithStatus(consts.StatusNoContent)
		return
	}

	c.Next(ctx)
}

// localHostEffectMiddleware prevents authenticated LAN clients from opening
// windows on the machine that runs Text Adventure. Remote browsers cannot usefully
// select a server-local absolute path in any case.
func localHostEffectMiddleware(ctx context.Context, c *app.RequestContext) {
	if !isLocalClientIP(requestClientIP(c)) {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.localHostEffect")
		return
	}
	c.Next(ctx)
}

// The key travels as the response code so a client can tell these refusals
// apart; without it every middleware refusal reaches the client as a bare
// "http.<status>" and cannot be translated or acted on.
func abortWithLocalizedError(c *app.RequestContext, status int, key string) {
	message := i18n.FromHeader(localeHeader(c)).T(key)
	c.AbortWithStatusJSON(status, map[string]string{"error": message, "code": key})
}

func localeHeader(c *app.RequestContext) string {
	for _, name := range []string{"X-TA-Locale", "X-Denova-Locale", "X-Nova-Locale"} {
		if header := strings.TrimSpace(string(c.Request.Header.Peek(name))); header != "" {
			return header
		}
	}
	return ""
}

func requestClientIP(c *app.RequestContext) string {
	remote := directClientIP(c)
	if isLocalClientIP(remote) {
		if forwarded := forwardedClientIP(string(c.Request.Header.Peek("X-Forwarded-For"))); forwarded != "" {
			return forwarded
		}
	}
	return remote
}

func directClientIP(c *app.RequestContext) string {
	if addr := c.RemoteAddr(); addr != nil {
		host, _, err := net.SplitHostPort(strings.TrimSpace(addr.String()))
		if err == nil {
			return host
		}
		if addr.String() != "" {
			return addr.String()
		}
	}
	return c.ClientIP()
}

func forwardedClientIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(part)
		if net.ParseIP(value) != nil {
			return value
		}
	}
	return ""
}

func isLocalClientIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsUnspecified()
}

// updateAdmissionMiddleware holds a request admission through its handler.
// Apply itself must acquire the exclusive side rather than a shared admission.
func updateAdmissionMiddleware(application *runtimeapp.App) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if string(c.Path()) == "/api/update/apply" {
			c.Next(ctx)
			return
		}
		release, err := application.AcquireUpdateUse()
		if err != nil {
			abortWithLocalizedError(c, consts.StatusConflict, "api.update.busy")
			return
		}
		defer release()
		c.Next(ctx)
	}
}
