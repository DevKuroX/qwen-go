"""
account_pool.py v2 — High-concurrency account scheduling engine

Min-Heap scheduling + 6-state lifecycle + circuit breaker + adaptive rate limiting + cold-start warmup + auto-replenishment
"""

import asyncio
import heapq
import hashlib
import json
import logging
import random
import time
from collections import deque
from typing import Optional, Any

log = logging.getLogger("qwenpi.account_pool")

# ─── Status constants ──────────────────────────────────────────
STATUS_VALID = "VALID"
STATUS_RATE_LIMITED = "RATE_LIMITED"
STATUS_SOFT_ERROR = "SOFT_ERROR"
STATUS_CIRCUIT_OPEN = "CIRCUIT_OPEN"
STATUS_HALF_OPEN = "HALF_OPEN"
STATUS_BANNED = "BANNED"
STATUS_PENDING_REFRESH = "PENDING_REFRESH"

BANNED_KEYWORDS = (
    "account has been banned", "account suspended", "account disabled",
    "violates our terms", "risk control", "permanently restricted",
    "forbidden by policy",
)

TRANSIENT_KEYWORDS = ("timeout", "connection reset", "connection refused",
                      "eof", "broken pipe", "temporary failure", "dns")


class Account:
    """Represents an upstream Qwen account."""

    __slots__ = (
        "email", "password", "token", "username",
        "status", "inflight",
        "rate_limited_until", "rate_limit_count",
        "consecutive_failures", "circuit_open_count", "circuit_open_until",
        "activation_pending", "last_error", "last_request_started",
        "rpm_window", "tpm_window",
        "learned_max_rpm", "learned_max_tpm",
        "created_at", "warmup_until",
        "_score", "_heap_idx",
    )

    def __init__(self, email: str, password: str = "", token: str = "",
                 username: str = "", status: str = STATUS_VALID):
        self.email = email
        self.password = password
        self.token = token
        self.username = username or email.split("@")[0]
        self.status = status
        self.inflight = 0
        self.rate_limited_until = 0.0
        self.rate_limit_count = 0
        self.consecutive_failures = 0
        self.circuit_open_count = 0
        self.circuit_open_until = 0.0
        self.activation_pending = False
        self.last_error = ""
        self.last_request_started = 0.0
        # Sliding windows
        self.rpm_window: deque = deque()       # timestamps
        self.tpm_window: deque = deque()       # (timestamp, token_count)
        # Adaptive learning
        self.learned_max_rpm: int = 50
        self.learned_max_tpm: int = 500_000
        # Cold start
        self.created_at: float = time.time()
        self.warmup_until: float = time.time() + 7200  # default 2h warmup
        # Heap scheduling
        self._score: float = 0.0
        self._heap_idx: int = 0

    # ── Sliding windows ─────────────────────────────
    def _clean_windows(self):
        now = time.time()
        cutoff = now - 60
        while self.rpm_window and self.rpm_window[0] < cutoff:
            self.rpm_window.popleft()
        while self.tpm_window and self.tpm_window[0][0] < cutoff:
            self.tpm_window.popleft()

    @property
    def rpm_1min(self) -> int:
        self._clean_windows()
        return len(self.rpm_window)

    @property
    def tpm_1min(self) -> int:
        self._clean_windows()
        return sum(t[1] for t in self.tpm_window)

    def record_request(self):
        self.rpm_window.append(time.time())

    def record_tokens(self, tokens: int):
        if tokens > 0:
            self.tpm_window.append((time.time(), tokens))

    # ── Cold-start rate limiting ────────────────────────────
    @property
    def effective_max_rpm(self) -> int:
        now = time.time()
        if now < self.warmup_until:
            elapsed = now - self.created_at
            if elapsed < 1800:       # first 30 min
                return max(5, self.learned_max_rpm // 10)
            elif elapsed < 3600:     # 30-60 min
                return max(10, self.learned_max_rpm // 5)
            elif elapsed < 7200:     # 1-2h
                return max(20, self.learned_max_rpm // 2)
        return self.learned_max_rpm

    @property
    def effective_max_inflight(self) -> int:
        now = time.time()
        if now < self.warmup_until:
            elapsed = now - self.created_at
            if elapsed < 1800:
                return 1
            elif elapsed < 7200:
                return 2
        return 3  # full speed

    # ── Score calculation ────────────────────────────
    def compute_score(self) -> float:
        if self.status not in (STATUS_VALID, STATUS_SOFT_ERROR, STATUS_HALF_OPEN):
            return float("inf")
        score = (
            self.inflight * 50
            + self.rpm_1min * 2
            + self.tpm_1min / 10000
            + self.consecutive_failures * 20
            + self.rate_limit_count * 10
        )
        if self.status == STATUS_SOFT_ERROR:
            score += 200
        if self.status == STATUS_HALF_OPEN:
            score += 100  # half-open probe priority is below VALID
        return score

    # ── Serialization ────────────────────────────────
    def to_dict(self) -> dict:
        return {
            "email": self.email, "password": self.password,
            "token": self.token, "username": self.username,
            "status": self.status, "inflight": self.inflight,
            "rate_limited_until": self.rate_limited_until,
            "rate_limit_count": self.rate_limit_count,
            "consecutive_failures": self.consecutive_failures,
            "circuit_open_count": self.circuit_open_count,
            "activation_pending": self.activation_pending,
            "last_error": self.last_error,
            "learned_max_rpm": self.learned_max_rpm,
            "learned_max_tpm": self.learned_max_tpm,
            "created_at": self.created_at,
            "warmup_until": self.warmup_until,
            "rpm_1min": self.rpm_1min,
            "tpm_1min": self.tpm_1min,
            # legacy compatibility
            "valid": self.status not in (STATUS_BANNED,),
            "status_code": self.status,
            "status_text": self.last_error,
        }

    @staticmethod
    def from_dict(d: dict) -> "Account":
        # Compatible with v1 format
        status = d.get("status", None)
        if status is None:
            status = STATUS_VALID if d.get("valid", True) else STATUS_SOFT_ERROR
        acc = Account(
            email=d.get("email", ""),
            password=d.get("password", ""),
            token=d.get("token", ""),
            username=d.get("username", ""),
            status=status,
        )
        acc.activation_pending = d.get("activation_pending", False)
        acc.last_error = d.get("last_error", "") or d.get("status_text", "")
        acc.rate_limit_count = d.get("rate_limit_count", 0)
        acc.consecutive_failures = d.get("consecutive_failures", 0)
        acc.circuit_open_count = d.get("circuit_open_count", 0)
        acc.learned_max_rpm = d.get("learned_max_rpm", 50)
        acc.learned_max_tpm = d.get("learned_max_tpm", 500_000)
        acc.created_at = d.get("created_at", time.time())
        acc.warmup_until = d.get("warmup_until", 0)
        return acc


# ─── Heap Entry ─────────────────────────────────────
class _HeapEntry:
    """Wrapper for heap comparison."""
    __slots__ = ("score", "counter", "acc")

    _counter = 0

    def __init__(self, acc: Account):
        _HeapEntry._counter += 1
        self.counter = _HeapEntry._counter
        self.acc = acc
        self.score = acc.compute_score()

    def __lt__(self, other):
        if self.score == other.score:
            return self.counter < other.counter
        return self.score < other.score


# ─── AccountPool v2 ─────────────────────────────────
class AccountPool:
    """Min-Heap scheduling + 6-state lifecycle management."""

    def __init__(self, accounts_db, settings=None):
        self.accounts_db = accounts_db
        self._settings = settings
        self._accounts: list[Account] = []
        self._heap: list[_HeapEntry] = []
        self._lock = asyncio.Lock()
        self._event = asyncio.Event()
        # Sticky map with LRU eviction
        from collections import OrderedDict
        self._sticky_map: OrderedDict = OrderedDict()
        self._sticky_max_size = 10_000
        # SSE event queue
        self._sse_events: asyncio.Queue = asyncio.Queue(maxsize=1000)
        # Replenishment state
        self._replenish_retry_count = 0
        self._replenish_failing_since: float = 0
        self._emergency_replenish_running = False
        self._register_func = None
        # Background tasks
        self._bg_tasks: list[asyncio.Task] = []

    # ── Initialization & persistence ────────────────────────
    async def load(self):
        data = await self.accounts_db.get()
        # Defensive: auto-flatten unexpectedly nested arrays like [[{...}]]
        while isinstance(data, list) and len(data) == 1 and isinstance(data[0], list):
            data = data[0]
        async with self._lock:
            self._accounts = []
            for item in (data or []):
                if isinstance(item, dict) and item.get("token"):
                    self._accounts.append(Account.from_dict(item))
            self._rebuild_heap()
            log.info(f"[AccountPool] Loaded {len(self._accounts)} accounts")
            self._event.set()

    async def save(self):
        async with self._lock:
            data = [acc.to_dict() for acc in self._accounts]
        await self.accounts_db.save(data)

    def _rebuild_heap(self):
        self._heap = [_HeapEntry(acc) for acc in self._accounts]
        heapq.heapify(self._heap)

    def start_background_tasks(self):
        """Start background daemon tasks (called at app startup)."""
        self._bg_tasks.append(asyncio.create_task(self._circuit_recovery_loop()))
        self._bg_tasks.append(asyncio.create_task(self._sticky_cleanup_loop()))
        self._bg_tasks.append(asyncio.create_task(self._token_expiry_cleanup_loop()))
        log.info("[AccountPool] Background tasks started")

    # ── Core scheduling ─────────────────────────────
    def _is_available(self, acc: Account, exclude: set[str] | None = None) -> bool:
        if exclude and acc.email in exclude:
            return False
        if acc.status not in (STATUS_VALID, STATUS_SOFT_ERROR, STATUS_HALF_OPEN):
            return False
        if acc.activation_pending:
            return False
        if acc.status == STATUS_HALF_OPEN and acc.inflight > 0:
            return False  # HALF_OPEN allows only 1 probe
        if acc.inflight >= acc.effective_max_inflight:
            return False
        if acc.rpm_1min >= acc.effective_max_rpm:
            return False
        now = time.time()
        if acc.rate_limited_until > now:
            return False
        if not acc.token:
            return False
        # JWT expiration check (avoid handing out expired tokens)
        if self._is_token_expired(acc.token, now):
            return False
        return True

    @staticmethod
    def _is_token_expired(token: str, now: float = 0) -> bool:
        """Quickly check whether a JWT token is expired or invalid."""
        if not token:
            return True
        try:
            import base64, json as _json
            parts = token.split(".")
            if len(parts) < 2:
                return True  # Not a JWT format -> treat as invalid
            payload = parts[1]
            payload += "=" * (4 - len(payload) % 4)
            decoded = _json.loads(base64.b64decode(payload))
            exp = decoded.get("exp", 0)
            if not exp:
                return True  # No exp claim -> treat as invalid
            if exp < (now or __import__("time").time()):
                return True
        except Exception:
            return True  # Parse failure -> treat as invalid
        return False

    async def acquire(self, exclude: set[str] | None = None,
                      sticky_email: str | None = None) -> Optional[Account]:
        """Acquire the least-loaded available account. Sticky preference is honored first."""
        async with self._lock:
            # 1. Sticky priority
            if sticky_email:
                for acc in self._accounts:
                    if acc.email == sticky_email and self._is_available(acc, exclude):
                        acc.inflight += 1
                        acc.last_request_started = time.time()
                        acc.record_request()
                        return acc

            # 2. Min-Heap scheduling
            # Rebuild heap to find the best candidate (cached scores in the heap may be stale)
            candidates = []
            for acc in self._accounts:
                if self._is_available(acc, exclude):
                    candidates.append(acc)

            if not candidates:
                return None

            # Pick the lowest score
            best = min(candidates, key=lambda a: a.compute_score())
            best.inflight += 1
            best.last_request_started = time.time()
            best.record_request()
            return best

    async def acquire_wait(self, timeout: float = 60,
                           exclude: set[str] | None = None,
                           sticky_email: str | None = None) -> Optional[Account]:
        deadline = time.time() + timeout
        while time.time() < deadline:
            acc = await self.acquire(exclude, sticky_email)
            if acc:
                return acc
            try:
                remaining = deadline - time.time()
                await asyncio.wait_for(self._event.wait(), timeout=min(2.0, remaining))
            except asyncio.TimeoutError:
                pass
            self._event.clear()
        return None

    def release(self, acc: Account, tokens_used: int = 0):
        """Release the account and update statistics windows."""
        if acc.inflight > 0:
            acc.inflight -= 1
        if tokens_used > 0:
            acc.record_tokens(tokens_used)
        self._event.set()

    # ── Sticky session management ──────────────────────────
    def set_sticky(self, conversation_id: str, email: str, chat_id: str):
        if len(self._sticky_map) >= self._sticky_max_size:
            self._sticky_map.popitem(last=False)
        self._sticky_map[conversation_id] = (email, chat_id, time.time())
        self._sticky_map.move_to_end(conversation_id)

    def get_sticky(self, conversation_id: str) -> Optional[tuple[str, str]]:
        entry = self._sticky_map.get(conversation_id)
        if entry:
            email, chat_id, ts = entry
            if time.time() - ts < 1800:
                self._sticky_map[conversation_id] = (email, chat_id, time.time())
                self._sticky_map.move_to_end(conversation_id)
                return email, chat_id
            else:
                del self._sticky_map[conversation_id]
        return None

    def remove_sticky(self, conversation_id: str):
        self._sticky_map.pop(conversation_id, None)

    async def _sticky_cleanup_loop(self):
        while True:
            await asyncio.sleep(300)
            now = time.time()
            expired = [k for k, (_, _, ts) in self._sticky_map.items() if now - ts > 1800]
            for k in expired:
                del self._sticky_map[k]
            # LRU cap
            if len(self._sticky_map) > 10000:
                sorted_keys = sorted(self._sticky_map, key=lambda k: self._sticky_map[k][2])
                for k in sorted_keys[:len(self._sticky_map) - 10000]:
                    del self._sticky_map[k]

    async def _token_expiry_cleanup_loop(self):
        """Periodically purge accounts with expired JWTs, silently without notifying users."""
        # Run an immediate cleanup at startup
        await self._cleanup_expired_tokens()
        while True:
            await asyncio.sleep(600)  # then check every 10 minutes
            await self._cleanup_expired_tokens()

    async def _cleanup_expired_tokens(self):
        """Run a single expired-token cleanup pass."""
        now = time.time()
        expired_emails = []
        async with self._lock:
            for acc in self._accounts:
                if self._is_token_expired(acc.token, now):
                    expired_emails.append(acc.email)
        if expired_emails:
            async with self._lock:
                self._accounts = [a for a in self._accounts if a.email not in set(expired_emails)]
                self._rebuild_heap()
            await self.save()
            log.info(f"[AccountPool] Auto-cleaned {len(expired_emails)} expired account(s)")

    # ── Unified error handling ─────────────────────────
    def mark_error(self, acc: Account, error_type: str, msg: str = ""):
        """Unified error entry point. error_type: transient|rate_limit|auth|soft|ban"""
        msg_lower = msg.lower()

        # Transient errors — do not penalize the account
        if error_type == "transient" or any(kw in msg_lower for kw in TRANSIENT_KEYWORDS):
            log.debug(f"[Pool] Transient error, no penalty {acc.email}: {msg[:100]}")
            return

        # 429 rate limited
        if error_type == "rate_limit" or "429" in msg or "too many" in msg_lower:
            self._mark_rate_limited(acc, msg)
            return

        # Ban detection
        if error_type == "ban" or any(kw in msg_lower for kw in BANNED_KEYWORDS):
            self.mark_banned(acc, msg)
            return

        # 401 auth
        if error_type == "auth" or "401" in msg or "unauthorized" in msg_lower:
            acc.status = STATUS_PENDING_REFRESH
            acc.last_error = msg[:200]
            log.warning(f"[Pool] Auth failed, token refresh required: {acc.email}")
            self._event.set()
            return

        # Soft error
        acc.consecutive_failures += 1
        acc.last_error = msg[:200]

        if acc.consecutive_failures >= 5:
            self._open_circuit(acc)
        else:
            acc.status = STATUS_SOFT_ERROR
            log.warning(f"[Pool] Soft error #{acc.consecutive_failures}: {acc.email} — {msg[:100]}")

        self._event.set()

    def mark_success(self, acc: Account):
        """Request succeeded; reset error counters."""
        if acc.status == STATUS_HALF_OPEN:
            log.info(f"[Pool] Probe succeeded, circuit recovered: {acc.email}")
            acc.circuit_open_count = 0
        acc.consecutive_failures = 0
        acc.status = STATUS_VALID
        acc.last_error = ""

    def _mark_rate_limited(self, acc: Account, msg: str = ""):
        acc.rate_limit_count += 1
        msg_lower = msg.lower()

        # Detect daily quota cap (requires a longer cooldown)
        is_daily_limit = any(kw in msg_lower for kw in (
            "daily", "usage limit", "usage cap", "daily limit",
        ))

        if is_daily_limit:
            # Daily cap → cooldown 1 hour
            cooldown = 3600
            log.warning(f"[Pool] Daily limit hit for {acc.email}, cooldown {cooldown}s (1h)")
        else:
            # Standard rate limit → exponential backoff: 60 → 120 → 300 → 600 → 1800 (cap 30min)
            cooldowns = [60, 120, 300, 600, 1800]
            idx = min(acc.rate_limit_count - 1, len(cooldowns) - 1)
            cooldown = cooldowns[idx]

        acc.rate_limited_until = time.time() + cooldown
        acc.status = STATUS_RATE_LIMITED
        acc.last_error = msg[:200]

        # Adaptive learning: lower the RPM ceiling
        current_rpm = acc.rpm_1min
        if current_rpm < acc.learned_max_rpm:
            acc.learned_max_rpm = max(5, int(current_rpm * 0.8))
            log.info(f"[Pool] Adaptive RPM lowered: {acc.email} → {acc.learned_max_rpm}")

        log.warning(f"[Pool] Rate-limited {acc.email}, cooldown {cooldown}s (count {acc.rate_limit_count})")
        self._event.set()

    def _open_circuit(self, acc: Account):
        acc.circuit_open_count += 1
        # Exponential backoff: 60 → 120 → 240 → 480 → 960 → 1800 (cap 30min)
        cooldown = min(60 * (2 ** (acc.circuit_open_count - 1)), 1800)
        acc.circuit_open_until = time.time() + cooldown
        acc.status = STATUS_CIRCUIT_OPEN
        log.warning(f"[Pool] Circuit opened: {acc.email}, cooldown {cooldown}s (count {acc.circuit_open_count})")
        self._event.set()

    def mark_banned(self, acc: Account, msg: str = ""):
        acc.status = STATUS_BANNED
        acc.last_error = msg[:200]
        log.error(f"[Pool] Account banned: {acc.email} — {msg[:100]}")
        # Push SSE event to notify the frontend
        self._push_event("account_banned", f"Account {acc.email} has been banned")
        self._event.set()

    def mark_valid(self, acc: Account):
        acc.status = STATUS_VALID
        acc.consecutive_failures = 0
        acc.circuit_open_count = 0
        acc.activation_pending = False
        acc.rate_limited_until = 0
        acc.last_error = ""
        self._event.set()

    # ── Circuit recovery ────────────────────────────
    async def _circuit_recovery_loop(self):
        while True:
            await asyncio.sleep(30)
            now = time.time()
            for acc in self._accounts:
                # CIRCUIT_OPEN → HALF_OPEN
                if acc.status == STATUS_CIRCUIT_OPEN and now >= acc.circuit_open_until:
                    acc.status = STATUS_HALF_OPEN
                    log.info(f"[Pool] Circuit half-open: {acc.email}")
                    self._event.set()
                # RATE_LIMITED auto-recovery
                if acc.status == STATUS_RATE_LIMITED and now >= acc.rate_limited_until:
                    acc.status = STATUS_VALID
                    log.info(f"[Pool] Rate limit cleared: {acc.email}")
                    self._event.set()
                # Adaptive probing (slow recovery if no 429 for >1h)
                if acc.status == STATUS_VALID and acc.rate_limit_count > 0:
                    if now - acc.rate_limited_until > 3600:
                        old = acc.learned_max_rpm
                        acc.learned_max_rpm = min(acc.learned_max_rpm + 5, 200)
                        if acc.learned_max_rpm != old:
                            log.debug(f"[Pool] Adaptive RPM probe up: {acc.email} {old} → {acc.learned_max_rpm}")

    # ── SSE event push ──────────────────────────
    def _push_event(self, event_type: str, message: str, **extra):
        event = {"type": event_type, "message": message, "timestamp": time.time(), **extra}
        try:
            self._sse_events.put_nowait(event)
        except asyncio.QueueFull:
            pass  # drop the oldest

    async def get_sse_event(self, timeout: float = 30) -> Optional[dict]:
        try:
            return await asyncio.wait_for(self._sse_events.get(), timeout=timeout)
        except asyncio.TimeoutError:
            return None

    # ── Replenishment ─────────────────────────────────
    def count_valid(self) -> int:
        return sum(1 for a in self._accounts
                   if a.status in (STATUS_VALID, STATUS_SOFT_ERROR, STATUS_HALF_OPEN,
                                   STATUS_RATE_LIMITED, STATUS_PENDING_REFRESH))

    def count_banned(self) -> int:
        return sum(1 for a in self._accounts if a.status == STATUS_BANNED)

    async def start_replenishment_loop(self, register_func):
        """Start the replenishment daemon. register_func = async def(count, concurrency) -> int(success_count)."""
        self._register_func = register_func  # keep reference for emergency replenishment
        while True:
            # Sleep in 10s slices and recheck AUTO_REPLENISH each tick so toggles take effect quickly
            for _ in range(6):
                await asyncio.sleep(10)
                from lib.config import settings as _cfg
                if not getattr(_cfg, "AUTO_REPLENISH", False):
                    break  # disabled — skip the remaining wait; the tick below will return immediately
            try:
                await self._replenishment_tick(register_func)
            except Exception as e:
                log.error(f"[Replenish] Unknown error: {e}")


    def trigger_emergency_replenish(self):
        """Trigger emergency replenishment when all accounts are rate-limited / exhausted (background, non-blocking)."""
        from lib.config import settings
        if not getattr(settings, "AUTO_REPLENISH_ON_EXHAUST", False):
            log.info("[EmergencyReplenish] Emergency replenishment is disabled, skipping")
            return
        if self._register_func is None:
            log.warning("[EmergencyReplenish] register_func not injected, cannot trigger emergency replenishment")
            return
        if self._emergency_replenish_running:
            log.info("[EmergencyReplenish] Emergency replenishment already running, skipping duplicate trigger")
            return

        count = getattr(settings, "REPLENISH_EXHAUST_COUNT", 10)
        concurrency = getattr(settings, "REPLENISH_EXHAUST_CONCURRENCY", 3)
        log.warning(f"[EmergencyReplenish] All accounts exhausted; triggering emergency registration of {count} accounts")
        self._push_event("replenish_started", f"All accounts have hit their rate limit; emergency registration of {count} new accounts is in progress...")

        async def _do_emergency():
            self._emergency_replenish_running = True
            try:
                registered = await self._register_func(count, concurrency)
                if registered > 0:
                    log.info(f"[EmergencyReplenish] ✅ Emergency replenishment succeeded; registered {registered} account(s)")
                    self._push_event("replenish_success", f"Emergency replenishment succeeded: registered {registered} new account(s); you can retry image generation")
                    await self.load()
                else:
                    log.warning("[EmergencyReplenish] Emergency replenishment finished but registered 0 accounts")
                    self._push_event("replenish_error", "Emergency replenishment finished but no accounts were registered; please check the mail service configuration")
            except Exception as e:
                log.error(f"[EmergencyReplenish] Emergency replenishment failed: {e}")
                self._push_event("replenish_error", f"Emergency replenishment failed: {str(e)[:150]}")
            finally:
                self._emergency_replenish_running = False

        asyncio.create_task(_do_emergency())

    async def _replenishment_tick(self, register_func):
        from lib.config import settings
        if not getattr(settings, "AUTO_REPLENISH", False):
            self._replenish_retry_count = 0
            return

        valid = self.count_valid()
        target = getattr(settings, "REPLENISH_TARGET", 30)
        if valid >= target:
            self._replenish_retry_count = 0
            self._replenish_failing_since = 0
            return

        needed = target - valid
        concurrency = getattr(settings, "REPLENISH_CONCURRENCY", 3)
        log.info(f"[Replenish] Need to add {needed} account(s) (current {valid}/{target})")

        try:
            registered = await register_func(needed, concurrency)
            if registered > 0:
                log.info(f"[Replenish] Successfully registered {registered} account(s)")
                self._push_event("replenish_success", f"Auto-replenishment succeeded: registered {registered} account(s)")
                self._replenish_retry_count = 0
                self._replenish_failing_since = 0
                await self.load()  # reload accounts
        except Exception as e:
            self._replenish_retry_count += 1
            if self._replenish_failing_since == 0:
                self._replenish_failing_since = time.time()

            elapsed = time.time() - self._replenish_failing_since
            error_msg = str(e)[:200]
            log.error(f"[Replenish] Registration failed (attempt {self._replenish_retry_count}): {error_msg}")

            if elapsed < 1800:  # keep retrying within 30 minutes
                retry_in = 300  # retry after 5 minutes
                attempts_left = max(0, 6 - self._replenish_retry_count)
                self._push_event("replenish_error", f"Auto-replenishment failed: {error_msg}",
                                 retry_in=retry_in, attempts_left=attempts_left)
            else:
                # Auto-stop after 30 minutes
                settings.AUTO_REPLENISH = False
                self._replenish_retry_count = 0
                self._replenish_failing_since = 0
                self._push_event("replenish_stopped",
                                 "Auto-replenishment stopped (registrations kept failing for 30 minutes; please check the mail service configuration)")
                log.error("[Replenish] 30-minute retry budget exhausted; auto-replenishment stopped")

    # ── Account management ─────────────────────────────
    async def add_account(self, email: str, password: str = "", token: str = "") -> Account:
        async with self._lock:
            for existing in self._accounts:
                if existing.email == email:
                    existing.token = token or existing.token
                    existing.password = password or existing.password
                    existing.status = STATUS_VALID
                    existing.consecutive_failures = 0
                    return existing
            acc = Account(email=email, password=password, token=token)
            self._accounts.append(acc)
        await self.save()
        self._event.set()
        return acc

    async def remove_account(self, email: str, manual: bool = True) -> bool:
        """Remove an account. manual=True means user-initiated and will not trigger replenishment."""
        async with self._lock:
            before = len(self._accounts)
            self._accounts = [a for a in self._accounts if a.email != email]
            removed = len(self._accounts) < before
        if removed:
            await self.save()
            if not manual:
                self._push_event("account_removed", f"Account {email} was auto-removed")
        return removed

    def get_account_by_email(self, email: str) -> Optional[Account]:
        for acc in self._accounts:
            if acc.email == email:
                return acc
        return None

    def all_accounts(self) -> list[Account]:
        return list(self._accounts)

    # ── Backpressure signal ─────────────────────────────
    def pressure(self) -> float:
        """Return system pressure between 0.0 and 1.0."""
        total = len(self._accounts)
        if total == 0:
            return 1.0
        valid = sum(1 for a in self._accounts if a.status in (STATUS_VALID, STATUS_SOFT_ERROR))
        if valid == 0:
            return 1.0
        busy = sum(1 for a in self._accounts
                   if a.status in (STATUS_VALID, STATUS_SOFT_ERROR) and a.inflight > 0)
        return busy / valid

    # ── Status summary ─────────────────────────────
    def status(self) -> dict:
        now = time.time()
        total = len(self._accounts)
        valid = sum(1 for a in self._accounts if a.status == STATUS_VALID)
        soft_error = sum(1 for a in self._accounts if a.status == STATUS_SOFT_ERROR)
        rate_limited = sum(1 for a in self._accounts if a.status == STATUS_RATE_LIMITED)
        circuit_open = sum(1 for a in self._accounts if a.status in (STATUS_CIRCUIT_OPEN, STATUS_HALF_OPEN))
        banned = sum(1 for a in self._accounts if a.status == STATUS_BANNED)
        pending = sum(1 for a in self._accounts if a.status == STATUS_PENDING_REFRESH)
        in_use = sum(1 for a in self._accounts if a.inflight > 0)
        waiting = sum(a.inflight for a in self._accounts)
        activation_pending = sum(1 for a in self._accounts if a.activation_pending)
        return {
            "total": total, "valid": valid, "soft_error": soft_error,
            "rate_limited": rate_limited, "circuit_open": circuit_open,
            "banned": banned, "pending_refresh": pending,
            "in_use": in_use, "waiting": waiting,
            "activation_pending": activation_pending,
            "pressure": round(self.pressure(), 2),
            # v1 compatibility
            "invalid": banned + circuit_open,
        }

    def pool_stats(self) -> list[dict]:
        """Return real-time RPM/TPM/status info for each account."""
        return [
            {
                "email": acc.email,
                "status": acc.status,
                "inflight": acc.inflight,
                "rpm_1min": acc.rpm_1min,
                "tpm_1min": acc.tpm_1min,
                "learned_max_rpm": acc.learned_max_rpm,
                "consecutive_failures": acc.consecutive_failures,
                "rate_limit_count": acc.rate_limit_count,
                "score": round(acc.compute_score(), 1),
                "last_error": acc.last_error,
            }
            for acc in self._accounts
        ]
