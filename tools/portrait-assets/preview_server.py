"""Serve only the approved portrait review bundle on loopback for browser review."""
from __future__ import annotations
import argparse, json, mimetypes
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import unquote, urlsplit
from scan import safe_resolve

PROJECT_ROOT = Path(__file__).resolve().parents[2]
WORK_ROOT = PROJECT_ROOT / "work" / "luna6-portrait-assets"
MEDIA_ROOTS = {
    "mother": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\头像母图"),
    "h3": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\H3生成"),
}

def create_handler():
    inventory = json.loads(safe_resolve(WORK_ROOT, "inventory.json").read_text(encoding="utf-8"))
    media = {(a["source"], a["relative_path"]): a for a in inventory.get("media", [])}
    generated = {a["relative_path"]: a for a in inventory.get("generated_media", [])}
    thumbnails = {thumb for collection in (inventory.get("media", []), inventory.get("generated_media", [])) for asset in collection for thumb in asset.get("thumbnails", {}).values()}

    class Handler(BaseHTTPRequestHandler):
        server_version = "PortraitReview/1"
        def log_message(self, fmt, *args):
            pass
        def do_HEAD(self):
            self._serve(False)
        def do_GET(self):
            self._serve(True)
        def _serve(self, body):
            parsed = urlsplit(self.path)
            route = unquote(parsed.path)
            if parsed.query or "\\" in route or any(part in {".", ".."} for part in route.split("/")):
                self.send_error(404); return
            if route == "/review.html":
                target = safe_resolve(WORK_ROOT, "review.html")
                mime = "text/html; charset=utf-8"
            elif route.startswith("/derived/"):
                rel = route[1:]
                if rel not in thumbnails: self.send_error(404); return
                target = safe_resolve(WORK_ROOT, rel)
                mime = mimetypes.guess_type(target.name)[0] or "application/octet-stream"
            elif route.startswith("/media/legacy/"):
                remainder = route.removeprefix("/media/legacy/")
                source, separator, rel = remainder.partition("/")
                if not separator or source not in MEDIA_ROOTS or not rel: self.send_error(404); return
                asset = media.get((source, rel))
                if not asset: self.send_error(404); return
                target = safe_resolve(MEDIA_ROOTS[source], rel)
                mime = asset["media_type"]
            elif route.startswith("/media/generated/"):
                rel = route.removeprefix("/media/generated/")
                asset = generated.get(rel)
                if not asset: self.send_error(404); return
                target = safe_resolve(PROJECT_ROOT, asset["workspace_relative_path"])
                mime = asset["media_type"]
            else:
                self.send_error(404); return
            size = target.stat().st_size
            start, end, status = 0, size - 1, 200
            range_header = self.headers.get("Range")
            if range_header and mime.startswith("video/"):
                try:
                    unit, spec = range_header.split("=", 1)
                    if unit != "bytes" or "," in spec: raise ValueError()
                    left, right = spec.split("-", 1)
                    if left:
                        start = int(left); end = min(int(right), size - 1) if right else size - 1
                    else:
                        length = int(right); start = max(0, size - length)
                    if start < 0 or end < start or start >= size: raise ValueError()
                    status = 206
                except ValueError:
                    self.send_response(416); self.send_header("Content-Range", f"bytes */{size}"); self.end_headers(); return
            self.send_response(status)
            self.send_header("Content-Type", mime)
            self.send_header("Content-Length", str(max(0, end - start + 1)))
            self.send_header("X-Content-Type-Options", "nosniff")
            if mime.startswith("video/"):
                self.send_header("Accept-Ranges", "bytes")
                if status == 206: self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
            self.end_headers()
            if body:
                with target.open("rb") as stream:
                    stream.seek(start)
                    remaining = end - start + 1
                    while remaining:
                        chunk = stream.read(min(1024 * 1024, remaining))
                        if not chunk: break
                        self.wfile.write(chunk); remaining -= len(chunk)
    return Handler

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, required=True)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", args.port), create_handler())
    print(f"Portrait review preview listening on http://127.0.0.1:{args.port}/review.html", flush=True)
    try: server.serve_forever()
    except KeyboardInterrupt: pass
    finally: server.server_close()
if __name__ == "__main__": main()
