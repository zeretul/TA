package api

import (
	"context"
	"log/slog"
	"net"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/hertz-contrib/gzip"

	"ta/config"
	"ta/internal/app"
	"ta/internal/update"
)

// Release uploads need multipart overhead above the archive limit. Hertz spills
// large multipart files to disk; individual handlers enforce their content limits.
const maxRequestBodyBytes = int(update.MaxLocalArchiveBytes) + (1 << 20)

// Server 包含 Hertz 引擎和应用运行时。
type Server struct {
	engine *hertzserver.Hertz
	app    *app.App
	port   string
	host   string
}

// NewServer 构造 HTTP 服务。
func NewServer(application *app.App, port string) *Server {
	return newServer(application, port, nil)
}

// NewServerWithListener constructs an HTTP server using an already reserved
// listener. Callers retain responsibility for choosing the listener address.
func NewServerWithListener(application *app.App, port string, listener net.Listener) *Server {
	return newServer(application, port, listener)
}

func newServer(application *app.App, port string, listener net.Listener) *Server {
	configureHertzLogging()
	remoteAccess := application.RemoteAccessConfig()
	host := config.HTTPListenHost(remoteAccess.AllowLANAccess)
	s := &Server{
		app:  application,
		port: port,
		host: host,
	}

	options := []hertzconfig.Option{
		hertzserver.WithHostPorts(host + ":" + port),
		hertzserver.WithMaxRequestBodySize(maxRequestBodyBytes),
	}
	if listener != nil {
		options = append(options, hertzserver.WithListener(listener))
	}
	h := hertzserver.Default(options...)
	// The gzip middleware buffers body streams before compressing them. Exclude
	// every SSE route at the server boundary so browsers can consume events as
	// they arrive even when their automatic Accept-Encoding header includes gzip.
	//
	// It is registered first, outside the observability middleware, because
	// that middleware reads the finished body back to add the diagnostics
	// envelope (request id, domain code, build context) and to redact local
	// paths. A browser sends Accept-Encoding: gzip on failing requests too, and
	// when compression ran inside it the read-back saw only gzip bytes: the
	// client received the raw error with no request id, no code, and an
	// unredacted absolute path. Compression now happens after the envelope is
	// in place, so both forms of the response carry the same diagnostics.
	h.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathRegexes([]string{
		`^/api/.*/stream(?:\?.*)?$`,
		`^/api/(?:chat|interactive/chat|projects/[^/]+/agent-chat/chat|workspace/events)(?:\?.*)?$`,
		// A clip is already compressed, is tens of megabytes, and is read by the
		// player a range at a time. Compressing it would buffer the whole file in
		// memory before the first byte reached the browser and would break the
		// byte spans a range request asked for.
		`^/api/media/[^/]+$`,
		`^/api/interactive/stories/[^/]+/intimacy/media/[^/]+$`,
		`^/api/portraits/[^/]+$`,
		`^/api/catalog-art/[^/]+$`,
	})))
	h.Use(requestObservabilityMiddleware)
	h.Use(corsMiddleware)
	accessGate := newRemoteAccessGate(application.RemoteAccessConfig, port)
	h.Use(accessGate.middleware)
	accessGate.registerRoutes(h)
	s.registerRoutes(h)
	s.engine = h
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run() {
	slog.InfoContext(context.Background(), "http_server_started", "host", s.host, "port", s.port)
	s.engine.Spin()
}
