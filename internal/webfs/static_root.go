package webfs

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// ResolveDiskRoot returns the same validated frontend directory for HTTP and
// scenario export. It never extracts embedded data or exposes an unbuilt tree.
func ResolveDiskRoot() string {
	candidates := []string{}
	for _, name := range []string{"TA_WEB_DIR", "DENOVA_WEB_DIR", "NOVA_WEB_DIR"} {
		if v := os.Getenv(name); v != "" {
			candidates = append(candidates, v)
			break
		}
	}
	candidates = append(candidates, "web")
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "web"),
			filepath.Join(exeDir, "..", "web"),
			filepath.Join(exeDir, "..", "..", "web"),
		)
	}
	for _, candidate := range candidates {
		root := NormalizeRoot(candidate)
		if root == "" {
			continue
		}
		// A checkout's web/index.html loads TypeScript that only Vite can serve.
		// LAN links use this server, so serve the checkout's build just like a
		// release bundle. Never fall back to exposing the uncompiled source tree.
		if _, err := os.Stat(filepath.Join(root, "src", "main.tsx")); err == nil {
			root = filepath.Join(root, "dist")
			if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
				slog.WarnContext(context.Background(), "[startup] Compiled frontend missing; run pnpm --dir web build to enable browser access", "path", root)
				continue
			}
		}
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			if _, err := os.Stat(filepath.Join(root, "index.html")); err == nil {
				return root
			}
		}
	}
	return ""
}

func ResolveRoot() string {
	if root := ResolveDiskRoot(); root != "" {
		return root
	}
	// Last resort: assets embedded into the binary (build tag "embedweb").
	// Lets a bare nova binary serve the frontend with no web/ directory on
	// disk — useful for go install / single-binary distribution. Extracts to
	// a temp dir the file-based static handler can serve from.
	if HasEmbedded() {
		root, err := ExtractEmbedded()
		if err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[startup] failed to extract embedded frontend assets; registering API routes only: %v", err))
			return ""
		}
		slog.InfoContext(context.Background(), fmt.Sprintf("[startup] disk Web directory not found; using embedded frontend assets: %s", root))
		return root
	}
	return ""
}

func NormalizeRoot(root string) string {
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return filepath.Clean(root)
}

// ReadAsset reads only frontend-relative resources, using the HTTP disk root
// first and the embedded build otherwise. Callers validate their asset namespace.
func ReadAsset(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, os.ErrInvalid
	}
	if root := ResolveDiskRoot(); root != "" {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	}
	if hasEmbedded {
		return fs.ReadFile(embeddedFS, "dist/"+name)
	}
	return nil, os.ErrNotExist
}
