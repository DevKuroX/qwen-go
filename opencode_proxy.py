#!/usr/bin/env python3
"""OpenAI-compatible proxy with auto-rotate session pool"""
import uuid, json, urllib.request, urllib.error, threading, time, http.server

UPSTREAM = "https://opencode.ai/zen/v1/chat/completions"
MODEL = "deepseek-v4-flash-free"

class SessionPool:
    def __init__(self, max_per_session=200):
        self.sessions = []
        self.idx = 0
        self.max_per_session = max_per_session
        self.lock = threading.Lock()
        self._add_session()

    def _add_session(self):
        s = {
            "id": "ses_" + uuid.uuid4().hex,
            "count": 0,
        }
        self.sessions.append(s)
        return s

    def get(self):
        with self.lock:
            for _ in range(len(self.sessions)):
                s = self.sessions[self.idx]
                self.idx = (self.idx + 1) % len(self.sessions)
                if s["count"] < self.max_per_session:
                    s["count"] += 1
                    return s["id"]
            # All sessions exhausted, add fresh one
            s = self._add_session()
            s["count"] = 1
            return s["id"]

    def mark_bad(self, sid):
        with self.lock:
            self.sessions = [s for s in self.sessions if s["id"] != sid]
            new_s = self._add_session()
            print(f"[pool] Session {sid[:20]} exhausted, rotated to {new_s['id'][:20]}")


pool = SessionPool(max_per_session=200)

def proxy_request(body_dict):
    session_id = pool.get()
    req_id = "msg_" + uuid.uuid4().hex[:20]
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer public",
        "User-Agent": "opencode/1.15.9 (Linux)",
        "x-opencode-session": session_id,
        "x-opencode-request": req_id,
    }
    # Force model to free model
    body_dict["model"] = MODEL
    if "stream" not in body_dict:
        body_dict["stream"] = False
    data = json.dumps(body_dict).encode()
    req = urllib.request.Request(UPSTREAM, data=data, headers=headers, method="POST")
    try:
        resp = urllib.request.urlopen(req, timeout=60)
        return resp.status, resp.read(), headers
    except urllib.error.HTTPError as e:
        body = e.read()
        if e.code == 429:
            pool.mark_bad(session_id)
            return proxy_request(body_dict)  # retry with new session
        return e.code, body, headers


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        clen = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(clen) if clen else b"{}"
        try:
            req_body = json.loads(body)
        except json.JSONDecodeError:
            self.send_error(400, "invalid json")
            return

        code, resp_body, req_headers = proxy_request(req_body)
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(resp_body)

    def do_GET(self):
        if self.path == "/v1/models":
            resp = json.dumps({
                "object": "list",
                "data": [{
                    "id": MODEL,
                    "object": "model",
                    "created": int(time.time()),
                    "owned_by": "opencode-free-proxy",
                }]
            })
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(resp.encode())
        else:
            self.send_error(404)

    def log_message(self, *a): pass


if __name__ == "__main__":
    port = 8080
    srv = http.server.HTTPServer(("0.0.0.0", port), Handler)
    print(f"OpenCode proxy running on http://0.0.0.0:{port}")
    print(f"Use like: openai base_url = http://localhost:{port}/v1")
    print(f"Pool size: {len(pool.sessions)}, max {pool.max_per_session}/session")
    srv.serve_forever()