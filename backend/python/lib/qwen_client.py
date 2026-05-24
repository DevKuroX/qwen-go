import asyncio
import json
import logging
import random
import time
import uuid
from typing import Optional, Any
from lib.account_pool import AccountPool, Account
from lib.config import settings
from lib.auth_resolver import AuthResolver

log = logging.getLogger("qwenpi.client")

AUTH_FAIL_KEYWORDS = ("token", "unauthorized", "expired", "forbidden", "401", "403", "invalid", "login", "activation", "pending activation", "not activated")
PENDING_ACTIVATION_KEYWORDS = ("pending activation", "please check your email", "not activated")
BANNED_KEYWORDS = ("banned", "suspended", "blocked", "disabled", "risk control", "violat", "forbidden by policy")

def _is_auth_error(error_msg: str) -> bool:
    msg = error_msg.lower()
    return any(keyword in msg for keyword in AUTH_FAIL_KEYWORDS)

def _is_pending_activation_error(error_msg: str) -> bool:
    msg = error_msg.lower()
    return any(keyword in msg for keyword in PENDING_ACTIVATION_KEYWORDS)

def _is_banned_error(error_msg: str) -> bool:
    msg = error_msg.lower()
    return any(keyword in msg for keyword in BANNED_KEYWORDS)

class QwenClient:
    def __init__(self, engine: Any, account_pool: AccountPool):
        self.engine = engine
        self.account_pool = account_pool
        self.auth_resolver = AuthResolver(account_pool)
        self.active_chat_ids: set[str] = set()  # chat_ids in active use; the GC must not reap them

    @staticmethod
    def _extract_urls_from_extra(extra: Any) -> list[str]:
        """Extract image URLs from the SSE delta.extra field.
        The extra field of a Qwen T2I response can contain:
          - {"wanx": {"image_list": [{"url": "..."}]}}
          - {"images": [{"url": "..."}]}
          - {"image_url": "..."}
          - top-level url/image string fields
        """
        urls: list[str] = []
        if not extra or not isinstance(extra, dict):
            return urls
        # wanx format
        wanx = extra.get("wanx") or {}
        if isinstance(wanx, dict):
            for item in wanx.get("image_list", []):
                u = item.get("url") or item.get("image_url") or ""
                if u and u.startswith("http"):
                    urls.append(u)
        # images list format
        for item in extra.get("images", []):
            u = (item.get("url") or item.get("image_url") or "") if isinstance(item, dict) else ""
            if u and u.startswith("http"):
                urls.append(u)
        # Top-level direct fields
        for key in ("image_url", "imageUrl", "url", "image"):
            v = extra.get(key, "")
            if isinstance(v, str) and v.startswith("http"):
                urls.append(v)
        return urls

    async def create_chat(self, token: str, model: str, chat_type: str = "t2t") -> str:
        ts = int(time.time())
        body = {"title": f"api_{ts}", "models": [model], "chat_mode": "normal",
                "chat_type": chat_type, "timestamp": ts}

        # Chat lifecycle calls also prefer the browser, which more closely matches a real-user path
        if hasattr(self.engine, "browser_engine") and getattr(self.engine, "browser_engine") is not None:
            r = await self.engine.browser_engine.api_call("POST", "/api/v2/chats/new", token, body)
            status = r.get("status")
            body_text = (r.get("body") or "").lower()
            should_fallback = (
                status == 0
                or status in (401, 403, 429)
                or "waf" in body_text
                or "<!doctype" in body_text
                or "forbidden" in body_text
                or "unauthorized" in body_text
            )
            if should_fallback:
                preview = (r.get("body") or "")[:160].replace("\n", "\\n")
                log.warning(f"[QwenClient] create_chat browser failed, falling back to default engine status={status} body_preview={preview!r}")
                r = await self.engine.api_call("POST", "/api/v2/chats/new", token, body)
        else:
            r = await self.engine.api_call("POST", "/api/v2/chats/new", token, body)
        if r["status"] == 429:
            raise Exception("429 Too Many Requests (Engine Queue Full)")

        body_text = r.get("body", "")
        if r["status"] != 200:
            body_lower = body_text.lower()
            if (r["status"] in (401, 403)
                    or "unauthorized" in body_lower or "forbidden" in body_lower
                    or "token" in body_lower or "login" in body_lower
                    or "401" in body_text or "403" in body_text):
                raise Exception(f"unauthorized: create_chat HTTP {r['status']}: {body_text[:100]}")
            raise Exception(f"create_chat HTTP {r['status']}: {body_text[:100]}")

        try:
            data = json.loads(body_text)
            if not data.get("success") or "id" not in data.get("data", {}):
                raise Exception("Qwen API returned error or missing id")
            return data["data"]["id"]
        except Exception as e:
            body_lower = body_text.lower()
            if any(kw in body_lower for kw in ("html", "login", "unauthorized", "activation",
                                                "pending", "forbidden", "token", "expired", "invalid")):
                raise Exception(f"unauthorized: account issue: {body_text[:200]}")
            raise Exception(f"create_chat parse error: {e}, body={body_text[:200]}")

    async def delete_chat(self, token: str, chat_id: str):
        if hasattr(self.engine, "browser_engine") and getattr(self.engine, "browser_engine") is not None:
            r = await self.engine.browser_engine.api_call("DELETE", f"/api/v2/chats/{chat_id}", token)
            status = r.get("status")
            body_text = (r.get("body") or "").lower()
            should_fallback = (
                status == 0
                or status in (401, 403, 429)
                or "waf" in body_text
                or "<!doctype" in body_text
                or "forbidden" in body_text
                or "unauthorized" in body_text
            )
            if should_fallback:
                preview = (r.get("body") or "")[:160].replace("\n", "\\n")
                log.warning(f"[QwenClient] delete_chat browser failed, falling back to default engine chat_id={chat_id} status={status} body_preview={preview!r}")
                await self.engine.api_call("DELETE", f"/api/v2/chats/{chat_id}", token)
            return
        await self.engine.api_call("DELETE", f"/api/v2/chats/{chat_id}", token)

    async def verify_token(self, token: str) -> bool:
        """Verify token validity via direct HTTP (no browser page needed)."""
        if not token:
            return False

        try:
            import httpx
            from lib.auth_resolver import BASE_URL

            # Forge browser fingerprints to avoid being blocked by Aliyun WAF
            headers = {
                "Authorization": f"Bearer {token}",
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                "Accept": "application/json, text/plain, */*",
                "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
                "Referer": "https://chat.qwen.ai/",
                "Origin": "https://chat.qwen.ai",
                "Connection": "keep-alive"
            }

            async with httpx.AsyncClient(timeout=15) as hc:
                resp = await hc.get(
                    f"{BASE_URL}/api/v1/auths/",
                    headers=headers,
                )
            if resp.status_code != 200:
                return False

            # Tolerate empty or non-JSON responses to avoid crashing on GFW interception or proxies returning fake 200 OKs
            try:
                data = resp.json()
                return data.get("role") == "user"
            except Exception as e:
                log.warning(f"[verify_token] JSON parse error (possibly intercepted or proxy issue): {e}, status={resp.status_code}, text={resp.text[:100]}")
                # If we hit Aliyun WAF interception, it usually means the direct httpx request is blocked, while the token itself may be fine.
                # Since this is meant as a fast check, when WAF blocks (HTML), assume the token is alive and let the browser engine handle it for real.
                if "aliyun_waf" in resp.text.lower() or "<!doctype" in resp.text.lower():
                    log.info(f"[verify_token] Hit a WAF interception page; passing through to the underlying headless browser engine.")
                    return True
                return False
        except Exception as e:
            log.warning(f"[verify_token] HTTP error: {e}")
            return False

    async def list_models(self, token: str) -> list:
        try:
            import httpx
            from lib.auth_resolver import BASE_URL

            headers = {
                "Authorization": f"Bearer {token}",
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                "Accept": "application/json, text/plain, */*",
                "Referer": "https://chat.qwen.ai/",
                "Origin": "https://chat.qwen.ai",
                "Connection": "keep-alive"
            }

            async with httpx.AsyncClient(timeout=10) as hc:
                resp = await hc.get(
                    f"{BASE_URL}/api/models",
                    headers=headers,
                )
            if resp.status_code != 200:
                return []
            try:
                return resp.json().get("data", [])
            except Exception as e:
                log.warning(f"[list_models] JSON parse error: {e}, status={resp.status_code}, text={resp.text[:100]}")
                return []
        except Exception:
            return []

    def _build_payload(self, chat_id: str, model: str, content: str,
                        has_custom_tools: bool = False,
                        enable_native_fc: Optional[bool] = None,
                        thinking: Optional[bool] = None) -> dict:
        ts = int(time.time())
        # has_custom_tools=True: disable thinking/search/plugins (applies to any tool-calling mode)
        # enable_native_fc: independently controls whether to enable Qwen's platform-native function_calling
        # thinking: explicit control of thinking mode (None=auto, True=force on, False=force off)
        if enable_native_fc is None:
            enable_native_fc = bool(has_custom_tools and settings.NATIVE_TOOL_PASSTHROUGH)
        # Thinking mode decision:
        # thinking=True → force thinking on
        # thinking=False → force thinking off (fast mode)
        # thinking=None → auto mode (off when tools are present, otherwise let Qwen decide automatically)
        if has_custom_tools:
            think_on = False
            think_mode = "off"
        elif thinking is True:
            think_on = True
            think_mode = "on"
        elif thinking is False:
            think_on = False
            think_mode = "off"
        else:
            # None = auto mode
            think_on = True
            think_mode = "Auto"
        feature_config = {
            "thinking_enabled": think_on,
            "output_schema": "phase",
            "research_mode": "normal",
            "auto_thinking": think_on and think_mode == "Auto",
            "thinking_mode": think_mode,
            "thinking_format": "summary",
            "auto_search": not has_custom_tools,
            "code_interpreter": not has_custom_tools,
            "function_calling": enable_native_fc,
            "plugins_enabled": False if has_custom_tools else True,
        }
        return {
            "stream": True, "version": "2.1", "incremental_output": True,
            "chat_id": chat_id, "chat_mode": "normal", "model": model, "parent_id": None,
            "messages": [{
                "fid": str(uuid.uuid4()), "parentId": None, "childrenIds": [str(uuid.uuid4())],
                "role": "user", "content": content, "user_action": "chat", "files": [],
                "timestamp": ts, "models": [model], "chat_type": "t2t",
                "feature_config": feature_config,
                "extra": {"meta": {"subChatType": "t2t"}}, "sub_chat_type": "t2t", "parent_id": None,
            }],
            "timestamp": ts,
        }

    def _build_video_payload(self, chat_id: str, model: str, prompt: str, aspect_ratio: str = "16:9") -> dict:
        """Build payload for Qwen text-to-video generation (chat_type=t2v).

        Qwen's video gen accepts the same aspect_ratio set as t2i; we just route to t2v.
        """
        ts = int(time.time())
        ratio_to_size: dict[str, str] = {
            "1:1":  "1024*1024",
            "16:9": "1280*720",
            "9:16": "720*1280",
            "4:3":  "1024*768",
            "3:4":  "768*1024",
        }
        px = ratio_to_size.get(aspect_ratio, "1280*720")
        px_x = px.replace("*", "x")
        w, h = px.split("*")
        feature_config = {
            "thinking_enabled": False,
            "output_schema": "phase",
            "auto_thinking": False,
            "thinking_mode": "off",
            "auto_search": False,
            "code_interpreter": False,
            "function_calling": False,
            "plugins_enabled": True,
            "video_generation": True,
            "default_aspect_ratio": aspect_ratio,
            "video_size": px,
            "t2v_size": px,
        }
        return {
            "stream": True,
            "version": "2.1",
            "incremental_output": True,
            "chat_id": chat_id,
            "chat_mode": "normal",
            "model": model,
            "parent_id": None,
            "messages": [{
                "fid": str(uuid.uuid4()),
                "parentId": None,
                "childrenIds": [str(uuid.uuid4())],
                "role": "user",
                # Upstream T2V prompt prefix
                "content": f"Generate video: {prompt}",
                "user_action": "chat",
                "files": [],
                "timestamp": ts,
                "models": [model],
                "chat_type": "t2v",
                "feature_config": feature_config,
                "extra": {"meta": {
                    "subChatType": "t2v",
                    "mode": "video_generation",
                    "aspectRatio": aspect_ratio,
                    "videoSize": px,
                    "size": px_x,
                    "width": int(w),
                    "height": int(h),
                    "video_generation_enabled": True,
                }},
                "sub_chat_type": "t2v",
                "parent_id": None,
            }],
            "timestamp": ts,
        }

    def _build_image_payload(self, chat_id: str, model: str, prompt: str, aspect_ratio: str = "1:1") -> dict:
        ts = int(time.time())
        # Map ratio → pixel dimensions (Wanx / Flux common sizes)
        ratio_to_size: dict[str, str] = {
            "1:1":  "1024*1024",
            "16:9": "1280*720",
            "9:16": "720*1280",
            "4:3":  "1024*768",
            "3:4":  "768*1024",
        }
        px = ratio_to_size.get(aspect_ratio, "1024*1024")  # e.g. "1280*720"
        px_x = px.replace("*", "x")                         # e.g. "1280x720"
        w, h = px.split("*")                                 # e.g. ("1280", "720")
        feature_config = {
            "thinking_enabled": False,
            "output_schema": "phase",
            "auto_thinking": False,
            "thinking_mode": "off",
            "auto_search": False,
            "code_interpreter": False,
            "function_calling": False,
            "plugins_enabled": True,
            "image_generation": True,
            "default_aspect_ratio": aspect_ratio,
            "image_size": px,
            "t2i_size": px,
        }
        return {
            "stream": True,
            "version": "2.1",
            "incremental_output": True,
            "chat_id": chat_id,
            "chat_mode": "normal",
            "model": model,
            "parent_id": None,
            "messages": [{
                "fid": str(uuid.uuid4()),
                "parentId": None,
                "childrenIds": [str(uuid.uuid4())],
                "role": "user",
                # Upstream T2I prompt prefix: Chinese for "Generate image:"
                "content": f"\u751f\u6210\u56fe\u7247\uff1a{prompt}",
                "user_action": "chat",
                "files": [],
                "timestamp": ts,
                "models": [model],
                "chat_type": "t2i",
                "feature_config": feature_config,
                "extra": {"meta": {
                    "subChatType": "t2i",
                    "mode": "image_generation",
                    "aspectRatio": aspect_ratio,   # "16:9"
                    "imageSize": px,               # "1280*720" (Wanx format)
                    "size": px_x,                  # "1280x720"
                    "width": int(w),
                    "height": int(h),
                    "image_generation_enabled": True,
                }},
                "sub_chat_type": "t2i",
                "parent_id": None,
            }],
            "timestamp": ts,
        }

    def parse_sse_chunk(self, chunk: str) -> list[dict]:
        events = []
        for line in chunk.splitlines():
            line = line.strip()
            if not line.startswith("data:"):
                continue
            data = line[5:].strip()
            if not data or data == "[DONE]":
                continue
            try:
                obj = json.loads(data)
                events.append(obj)
            except Exception:
                continue

        parsed = []
        for evt in events:
            if evt.get("choices"):
                delta = evt["choices"][0].get("delta", {})
                finish_reason = evt["choices"][0].get("finish_reason", "")
                # Extract tool_call_id (Qwen native tool_call event)
                extra = delta.get("extra", {})
                # In some versions tool_call_id is at the delta top level
                tc_id = (extra.get("tool_call_id")
                         or delta.get("tool_call_id")
                         or evt.get("tool_call_id")
                         or "tc_0")
                parsed.append({
                    "type": "delta",
                    "phase": delta.get("phase", "answer"),
                    "content": delta.get("content", ""),
                    "reasoning_content": delta.get("thought", "") or delta.get("reasoning_content", ""),
                    "status": delta.get("status", "") or finish_reason,
                    "extra": {**extra, "tool_call_id": tc_id},
                })
            elif evt.get("phase"):
                extra = evt.get("extra", {})
                tc_id = (extra.get("tool_call_id")
                         or evt.get("tool_call_id")
                         or "tc_0")
                parsed.append({
                    "type": "delta",
                    "phase": evt.get("phase", "answer"),
                    "content": evt.get("content", "") or evt.get("text", "") or "",
                    "reasoning_content": evt.get("thought", "") or evt.get("reasoning_content", ""),
                    "status": evt.get("status", ""),
                    "extra": {**extra, "tool_call_id": tc_id},
                })
        return parsed

    async def chat_stream_events_with_retry(self, model: str, content: str,
                                              has_custom_tools: bool = False,
                                              xml_mode: bool = False,
                                              exclude_accounts: Optional[set[str]] = None,
                                              thinking: Optional[bool] = None):
        """Transparent failover with retry: when upstream fails, automatically rotate accounts."""
        exclude = set(exclude_accounts or set())
        # xml_mode: tools are present but Qwen native FC is disabled; use XML prompt injection.
        # has_custom_tools must still be True so that thinking/search/plugins are disabled.
        effective_has_tools = has_custom_tools or xml_mode
        enable_native_fc = False if xml_mode else None  # None = use default logic
        for attempt in range(settings.MAX_RETRIES):
            acc = await self.account_pool.acquire_wait(timeout=60, exclude=exclude)
            if not acc:
                pool_status = self.account_pool.status()
                raise Exception(
                    "No available accounts in pool "
                    f"(total={pool_status['total']}, valid={pool_status['valid']}, "
                    f"invalid={pool_status['invalid']}, activation_pending={pool_status.get('activation_pending', 0)}, "
                    f"rate_limited={pool_status['rate_limited']}, in_use={pool_status['in_use']}, waiting={pool_status['waiting']})"
                )

            chat_id: Optional[str] = None
            try:
                log.info(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Acquired account: account={acc.email} model={model} tools={has_custom_tools} xml_mode={xml_mode} exclude={sorted(exclude)}")
                # Local throttling: skip jitter for large pools (>50 accounts); keep anti-detection jitter for smaller pools
                pool_size = len(self.account_pool._accounts) if hasattr(self.account_pool, '_accounts') else 100
                if pool_size < 50:
                    min_interval = max(0, settings.ACCOUNT_MIN_INTERVAL_MS) / 1000.0
                    now = time.time()
                    wait_s = max(0.0, (acc.last_request_started + min_interval) - now)
                    jitter_ms = random.randint(settings.REQUEST_JITTER_MIN_MS, settings.REQUEST_JITTER_MAX_MS)
                    wait_s += jitter_ms / 1000.0
                    if wait_s > 0:
                        log.debug(f"[Throttle] Account cooldown wait: account={acc.email} wait={wait_s:.2f}s (incl. jitter {jitter_ms}ms)")
                        await asyncio.sleep(wait_s)
                chat_id = await self.create_chat(acc.token, model)
                self.active_chat_ids.add(chat_id)
                payload = self._build_payload(chat_id, model, content, effective_has_tools, enable_native_fc, thinking)
                log.info(
                    f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Session created: account={acc.email} chat_id={chat_id} "
                    f"engine={self.engine.__class__.__name__} function_calling={payload['messages'][0]['feature_config'].get('function_calling')} "
                    f"thinking_enabled={payload['messages'][0]['feature_config'].get('thinking_enabled')}"
                )

                # First yield the chat_id and account to the consumer
                yield {"type": "meta", "chat_id": chat_id, "acc": acc}

                buffer = ""
                # Always use streaming: NativeBlock can be detected in real time and aborted early; no need to wait 3 minutes
                async for chunk_result in self.engine.fetch_chat(acc.token, chat_id, payload, buffered=False):
                    if chunk_result.get("status") == 429:
                        log.warning(f"[Local backpressure {attempt+1}/{settings.MAX_RETRIES}] Engine queue full: account={acc.email} chat_id={chat_id}")
                        raise Exception("local_backpressure: engine queue full")
                    if chunk_result.get("status") != 200 and chunk_result.get("status") != "streamed":
                        body_preview = (chunk_result.get("body", "")[:120]).replace("\n", "\\n")
                        log.warning(
                            f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Upstream chunk error: account={acc.email} chat_id={chat_id} "
                            f"status={chunk_result.get('status')} body_preview={body_preview!r}"
                        )
                        raise Exception(f"HTTP {chunk_result['status']}: {chunk_result.get('body', '')[:100]}")

                    if "chunk" in chunk_result:
                        buffer += chunk_result["chunk"]
                        while "\n" in buffer:
                            # If we see a line starting with data:, try to parse it even without \n\n
                            # Standard SSE is \n\n, but we want maximum reactivity
                            line, buffer = buffer.split("\n", 1)
                            if line.strip().startswith("data:"):
                                events = self.parse_sse_chunk(line)
                                for evt in events:
                                    yield {"type": "event", "event": evt}
                    elif "body" in chunk_result and chunk_result["body"] and chunk_result["body"] != "streamed":
                        buffer += chunk_result["body"]
                
                if buffer:
                    events = self.parse_sse_chunk(buffer)
                    for evt in events:
                        yield {"type": "event", "event": evt}
                log.info(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Stream complete: account={acc.email} chat_id={chat_id} buffered_chars={len(buffer)}")
                self.active_chat_ids.discard(chat_id)
                # v2: mark success with a rough token estimate
                self.account_pool.mark_success(acc)
                self.account_pool.release(acc, tokens_used=max(len(buffer) // 4, 100))
                return

            except Exception as e:
                if chat_id:
                    self.active_chat_ids.discard(chat_id)  # type: ignore[arg-type]
                err_msg = str(e).lower()
                should_save = False
                if "local_backpressure" in err_msg or "engine queue full" in err_msg:
                    acc.last_error = str(e)
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Local backpressure: account={acc.email} error={e}")
                elif "429" in err_msg or "rate limit" in err_msg or "too many" in err_msg:
                    self.account_pool.mark_error(acc, "rate_limit", str(e))
                    exclude.add(acc.email)
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Marked as rate-limited: account={acc.email} error={e}")
                elif _is_pending_activation_error(err_msg):
                    self.account_pool.mark_error(acc, "auth", str(e))
                    exclude.add(acc.email)
                    acc.activation_pending = True
                    should_save = True
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Marked as activation pending: account={acc.email} error={e}")
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                elif _is_banned_error(err_msg):
                    self.account_pool.mark_banned(acc, str(e))
                    exclude.add(acc.email)
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Marked as banned: account={acc.email} error={e}")
                elif _is_auth_error(err_msg):
                    self.account_pool.mark_error(acc, "auth", str(e))
                    exclude.add(acc.email)
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Marked as auth failure: account={acc.email} error={e}")
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                else:
                    self.account_pool.mark_error(acc, "transient", str(e))
                    log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Transient error: account={acc.email} error={e}")

                if should_save:
                    await self.account_pool.save()

                self.account_pool.release(acc)
                log.warning(f"[Retry {attempt+1}/{settings.MAX_RETRIES}] Account failed, preparing to retry: account={acc.email} error={e}")
                
        raise Exception(f"All {settings.MAX_RETRIES} attempts failed. Please check upstream accounts.")

    def _extract_urls_from_extra(self, extra: dict) -> list[str]:
        """Extract image URLs from the SSE event's extra field.

        Known formats:
        - extra.tool_result[0].image  (image_gen_tool finished event, primary path)
        - extra.image_url / extra.wanx_image_url / extra.imageUrl
        - extra.image_urls / extra.images / extra.imageUrls (lists)
        """
        urls = []
        if not extra or not isinstance(extra, dict):
            return urls

        # ① image_gen_tool finished event: extra.tool_result[].image
        tool_result = extra.get("tool_result")
        if isinstance(tool_result, list):
            for item in tool_result:
                if isinstance(item, dict):
                    for key in ("image", "url", "src", "imageUrl", "image_url"):
                        val = item.get(key)
                        if isinstance(val, str) and val.startswith("http"):
                            urls.append(val)
                elif isinstance(item, str) and item.startswith("http"):
                    urls.append(item)

        # ② Flat fields
        for key in ("image_url", "wanx_image_url", "imageUrl"):
            val = extra.get(key)
            if isinstance(val, str) and val.startswith("http"):
                urls.append(val)

        # ③ List fields
        for key in ("image_urls", "images", "imageUrls"):
            val = extra.get(key)
            if isinstance(val, list):
                for item in val:
                    if isinstance(item, str) and item.startswith("http"):
                        urls.append(item)
                    elif isinstance(item, dict):
                        for sub_key in ("url", "src", "image", "imageUrl"):
                            sub_val = item.get(sub_key)
                            if isinstance(sub_val, str) and sub_val.startswith("http"):
                                urls.append(sub_val)
        return urls

    async def image_generate_with_retry(self, model: str, prompt: str, aspect_ratio: str = "1:1", exclude_accounts: Optional[set[str]] = None) -> tuple[str, "Account", str]:
        """Call Qwen T2I to generate an image; returns (raw response text, account used, chat_id).

        Rotation strategy:
        - On each failure, the account is added to the exclude set so the next retry uses a different account.
        - RateLimited / daily limit reached → mark rate-limited, cool down for 30 minutes.
        - Auth errors → trigger auto-recovery.
        - Other errors → soft-error counter; circuit-break after 5 occurrences.
        """
        exclude = set(exclude_accounts or set())
        last_error: Optional[Exception] = None

        for attempt in range(settings.MAX_RETRIES):
            acc = await self.account_pool.acquire_wait(timeout=60, exclude=exclude)
            if not acc:
                pool_status = self.account_pool.status()
                raise Exception(
                    f"No available accounts in pool "
                    f"(valid={pool_status['valid']}, rate_limited={pool_status['rate_limited']}, "
                    f"excluded={len(exclude)})"
                )

            log.info(f"[T2I] Attempt {attempt+1}/{settings.MAX_RETRIES} using account {acc.email}")
            chat_id: Optional[str] = None
            try:
                chat_id = await self.create_chat(acc.token, model, chat_type="t2t")
                self.active_chat_ids.add(chat_id)
                payload = self._build_image_payload(chat_id, model, prompt, aspect_ratio)

                raw_body_parts: list[str] = []  # Keep raw SSE body for debugging
                answer_text = ""
                extra_urls: list[str] = []
                buffer = ""

                async for chunk_result in self.engine.fetch_chat(acc.token, chat_id, payload):
                    if chunk_result.get("status") == 429:
                        raise Exception("Engine Queue Full (429)")
                    if chunk_result.get("status") not in (200, "streamed"):
                        raise Exception(f"HTTP {chunk_result['status']}: {chunk_result.get('body', '')[:200]}")

                    # Append the raw text into the buffer
                    raw = ""
                    if "chunk" in chunk_result:
                        raw = chunk_result["chunk"]
                    elif "body" in chunk_result:
                        raw = chunk_result.get("body", "") or ""
                    if not raw:
                        continue

                    raw_body_parts.append(raw)
                    buffer += raw

                # Process the entire buffer (regardless of streaming or one-shot response)
                raw_body = "".join(raw_body_parts)
                log.info(f"[T2I] First 1000 chars of raw SSE body: {raw_body[:1000]!r}")

                # ── Detect error signals in the response ──
                raw_lower = raw_body.lower()
                is_rate_limited = (
                    "ratelimited" in raw_lower
                    or "rate_limited" in raw_lower
                    or "daily usage limit" in raw_lower
                    or "reached the da" in raw_lower       # "reached the daily usage limit"
                    # Chinese phrases for "request too frequent" / "usage limit reached" / "rate limit"
                    or "\u8bf7\u6c42\u8fc7\u4e8e\u9891\u7e41" in raw_body
                    or "\u4f7f\u7528\u4e0a\u9650" in raw_body
                    or "\u9891\u7387\u9650\u5236" in raw_body
                )
                is_api_error = (
                    '"success":false' in raw_body
                    or '"success": false' in raw_body
                )

                if is_rate_limited:
                    raise Exception(f"Qwen T2I RateLimit (daily): {raw_body[:300]}")
                if is_api_error and not extra_urls:
                    raise Exception(f"Qwen T2I API error: {raw_body[:300]}")

                for line in raw_body.splitlines():
                    line = line.strip()
                    if not line.startswith("data:"):
                        continue
                    data_str = line[5:].strip()
                    if not data_str or data_str == "[DONE]":
                        continue
                    try:
                        obj = json.loads(data_str)
                    except Exception:
                        continue

                    # Log every SSE event for diagnostics
                    log.info(f"[T2I-SSE] event: {json.dumps(obj, ensure_ascii=False)[:400]}")

                    # Extract from choices[0].delta
                    if obj.get("choices"):
                        delta = obj["choices"][0].get("delta", {})
                        content = delta.get("content", "")
                        phase = delta.get("phase", "answer")
                        extra = delta.get("extra", {})
                        log.info(f"[T2I-SSE] phase={phase!r} content_len={len(content)} content_preview={content[:100]!r}")
                        # Capture all text content
                        answer_text += content
                        # Capture image URLs from the extra field
                        extra_urls.extend(self._extract_urls_from_extra(extra))
                    elif obj.get("phase"):
                        # Top-level phase format
                        content = obj.get("content", "") or obj.get("text", "") or ""
                        phase = obj.get("phase", "")
                        extra = obj.get("extra", {})
                        log.info(f"[T2I-SSE] top-level phase={phase!r} content_len={len(content)} content_preview={content[:100]!r}")
                        answer_text += content
                        extra_urls.extend(self._extract_urls_from_extra(extra))

                # If extra contained image URLs, append them to answer_text as Markdown image syntax
                if extra_urls:
                    log.info(f"[T2I] Extracted {len(extra_urls)} image URLs from extra: {extra_urls}")
                    for url in extra_urls:
                        answer_text += f"\n![image]({url})"

                # If answer_text is empty, fall back to the raw body
                if not answer_text:
                    answer_text = raw_body

                self.active_chat_ids.discard(chat_id)
                log.info(f"[T2I] ✅ generation succeeded, account={acc.email}, response length={len(answer_text)}: {answer_text[:200]!r}")
                self.account_pool.mark_success(acc)
                self.account_pool.release(acc, tokens_used=max(len(answer_text) // 4, 200))
                return answer_text, acc, chat_id

            except Exception as e:
                if chat_id:
                    self.active_chat_ids.discard(chat_id)  # type: ignore[arg-type]
                
                err_msg = str(e)
                err_lower = err_msg.lower()
                last_error = e

                # ── Classify the error and mark the account ──
                if any(kw in err_lower for kw in ("ratelimit", "rate_limit", "rate limit", "daily", "usage limit", "429", "too many",
                                                    "\u4f7f\u7528\u4e0a\u9650", "\u9891\u7387\u9650\u5236")):
                    self.account_pool.mark_error(acc, "rate_limit", err_msg)
                    log.warning(f"[T2I] ⚠️ account {acc.email} hit the rate limit; marked rate-limited and rotating to next account")
                elif _is_pending_activation_error(err_lower):
                    self.account_pool.mark_error(acc, "auth", err_msg)
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                    log.warning(f"[T2I] ⚠️ account {acc.email} requires activation, triggering auto-recovery")
                elif _is_banned_error(err_lower):
                    self.account_pool.mark_banned(acc, err_msg)
                    log.warning(f"[T2I] 🚫 account {acc.email} has been banned")
                elif _is_auth_error(err_lower):
                    self.account_pool.mark_error(acc, "auth", err_msg)
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                    log.warning(f"[T2I] ⚠️ account {acc.email} authentication failed, triggering auto-recovery")
                else:
                    self.account_pool.mark_error(acc, "transient", err_msg)
                    log.warning(f"[T2I] ⚠️ account {acc.email} transient error: {err_msg[:150]}")

                self.account_pool.release(acc)
                # Add the failed account to the exclude set so the next round won't pick it again
                exclude.add(acc.email)
                log.info(f"[T2I] retry {attempt+1}/{settings.MAX_RETRIES}, exclude list: {exclude}")

        raise Exception(f"All {settings.MAX_RETRIES} T2I attempts failed. Last error: {last_error}")

    async def video_generate_with_retry(self, model: str, prompt: str, aspect_ratio: str = "16:9",
                                        exclude_accounts: Optional[set[str]] = None) -> tuple[str, "Account", str]:
        """Call Qwen T2V to generate a video; returns (raw response text, account used, chat_id).

        Mirrors `image_generate_with_retry` but routes through chat_type=t2v.
        Note: video generation can take significantly longer than image (30s-3min upstream).
        """
        exclude = set(exclude_accounts or set())
        last_error: Optional[Exception] = None

        for attempt in range(settings.MAX_RETRIES):
            acc = await self.account_pool.acquire_wait(timeout=60, exclude=exclude)
            if not acc:
                pool_status = self.account_pool.status()
                raise Exception(
                    f"No available accounts in pool "
                    f"(valid={pool_status['valid']}, rate_limited={pool_status['rate_limited']}, "
                    f"excluded={len(exclude)})"
                )

            log.info(f"[T2V] Attempt {attempt+1}/{settings.MAX_RETRIES} using account {acc.email}")
            chat_id: Optional[str] = None
            try:
                chat_id = await self.create_chat(acc.token, model, chat_type="t2v")
                self.active_chat_ids.add(chat_id)
                payload = self._build_video_payload(chat_id, model, prompt, aspect_ratio)

                raw_body_parts: list[str] = []
                answer_text = ""
                extra_urls: list[str] = []

                async for chunk_result in self.engine.fetch_chat(acc.token, chat_id, payload):
                    if chunk_result.get("status") == 429:
                        raise Exception("Engine Queue Full (429)")
                    if chunk_result.get("status") not in (200, "streamed"):
                        raise Exception(f"HTTP {chunk_result['status']}: {chunk_result.get('body', '')[:200]}")
                    raw = chunk_result.get("chunk") or chunk_result.get("body") or ""
                    if raw:
                        raw_body_parts.append(raw)

                raw_body = "".join(raw_body_parts)
                log.info(f"[T2V] First 1000 chars of raw SSE body: {raw_body[:1000]!r}")

                raw_lower = raw_body.lower()
                is_rate_limited = (
                    "ratelimited" in raw_lower
                    or "rate_limited" in raw_lower
                    or "daily usage limit" in raw_lower
                    or "reached the da" in raw_lower
                )
                is_unsupported = (
                    "not supported" in raw_lower
                    or "unsupported" in raw_lower
                    or "feature not available" in raw_lower
                )
                is_api_error = '"success":false' in raw_body or '"success": false' in raw_body

                if is_rate_limited:
                    raise Exception(f"Qwen T2V RateLimit (daily): {raw_body[:300]}")
                if is_unsupported:
                    raise Exception(f"Qwen T2V not available on this account: {raw_body[:300]}")
                if is_api_error and not extra_urls:
                    raise Exception(f"Qwen T2V API error: {raw_body[:300]}")

                for line in raw_body.splitlines():
                    line = line.strip()
                    if not line.startswith("data:"):
                        continue
                    data_str = line[5:].strip()
                    if not data_str or data_str == "[DONE]":
                        continue
                    try:
                        obj = json.loads(data_str)
                    except Exception:
                        continue

                    log.info(f"[T2V-SSE] event: {json.dumps(obj, ensure_ascii=False)[:400]}")

                    if obj.get("choices"):
                        delta = obj["choices"][0].get("delta", {})
                        content = delta.get("content", "")
                        extra = delta.get("extra", {})
                        answer_text += content
                        extra_urls.extend(self._extract_urls_from_extra(extra))
                    elif obj.get("phase"):
                        content = obj.get("content", "") or obj.get("text", "") or ""
                        extra = obj.get("extra", {})
                        answer_text += content
                        extra_urls.extend(self._extract_urls_from_extra(extra))

                if extra_urls:
                    log.info(f"[T2V] Extracted {len(extra_urls)} video URLs from extra: {extra_urls}")
                    for url in extra_urls:
                        answer_text += f"\n[video]({url})"

                if not answer_text:
                    answer_text = raw_body

                self.active_chat_ids.discard(chat_id)
                log.info(f"[T2V] ✅ generation succeeded, account={acc.email}, response length={len(answer_text)}")
                self.account_pool.mark_success(acc)
                self.account_pool.release(acc, tokens_used=max(len(answer_text) // 4, 200))
                return answer_text, acc, chat_id

            except Exception as e:
                if chat_id:
                    self.active_chat_ids.discard(chat_id)  # type: ignore[arg-type]
                err_msg = str(e)
                err_lower = err_msg.lower()
                last_error = e

                if any(kw in err_lower for kw in ("ratelimit", "rate_limit", "rate limit", "daily", "usage limit", "429", "too many")):
                    self.account_pool.mark_error(acc, "rate_limit", err_msg)
                elif _is_pending_activation_error(err_lower):
                    self.account_pool.mark_error(acc, "auth", err_msg)
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                elif _is_banned_error(err_lower):
                    self.account_pool.mark_banned(acc, err_msg)
                elif _is_auth_error(err_lower):
                    self.account_pool.mark_error(acc, "auth", err_msg)
                    asyncio.create_task(self.auth_resolver.auto_heal_account(acc))
                else:
                    self.account_pool.mark_error(acc, "transient", err_msg)

                self.account_pool.release(acc)
                exclude.add(acc.email)
                log.info(f"[T2V] retry {attempt+1}/{settings.MAX_RETRIES}, exclude list: {exclude}")

        raise Exception(f"All {settings.MAX_RETRIES} T2V attempts failed. Last error: {last_error}")

