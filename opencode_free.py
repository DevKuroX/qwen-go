#!/usr/bin/env python3
import uuid
import json
import urllib.request
import sys

BASE_URL = "https://opencode.ai/zen/v1/chat/completions"
MODEL = "deepseek-v4-flash-free"

def generate_session_id():
    return "ses_" + uuid.uuid4().hex

def generate_request_id():
    return "msg_" + uuid.uuid4().hex[:20]

class OpenCodeFree:
    def __init__(self, session_id=None):
        self.session_id = session_id or generate_session_id()

    def _request(self, payload):
        req_id = generate_request_id()
        headers = {
            "Content-Type": "application/json",
            "Authorization": "Bearer public",
            "User-Agent": "opencode/1.15.9 (Linux)",
            "x-opencode-session": self.session_id,
            "x-opencode-request": req_id,
        }
        data = json.dumps(payload).encode()
        req = urllib.request.Request(BASE_URL, data=data, headers=headers, method="POST")
        try:
            resp = urllib.request.urlopen(req, timeout=30)
            return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            body = e.read().decode()
            if e.code == 429:
                err = json.loads(body)
                raise Exception(f"Rate limited: {err.get('error', {}).get('message', 'unknown')}")
            raise

    def chat(self, message, stream=False):
        return self._request({
            "model": MODEL,
            "messages": [{"role": "user", "content": message}],
            "stream": stream,
        })

    def get_content(self, response):
        return response["choices"][0]["message"]["content"]

    def get_usage(self, response):
        return response.get("usage", {})


if __name__ == "__main__":
    oc = OpenCodeFree()
    print(f"Session ID: {oc.session_id}")
    print()

    while True:
        try:
            msg = input("You: ")
            if msg.lower() in ("exit", "quit", "q"):
                break
            r = oc.chat(msg)
            content = oc.get_content(r)
            print(f"AI: {content}")
            usage = oc.get_usage(r)
            print(f"[Tokens: {usage.get('total_tokens', '?')} | Cost: {r.get('cost', '?')}]")
        except KeyboardInterrupt:
            break
        except Exception as e:
            print(f"\n[ERROR: {e}]")
            if "Rate limited" in str(e):
                oc = OpenCodeFree()
                print(f"[Auto-rotate session: {oc.session_id}]")