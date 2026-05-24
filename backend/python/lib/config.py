import os
import json
import logging
import secrets
from pathlib import Path
from pydantic_settings import BaseSettings
from pydantic import field_validator
from typing import Dict, Set

BASE_DIR = Path(__file__).resolve().parent.parent.parent
DATA_DIR = BASE_DIR / "data"

class Settings(BaseSettings):
    # Service configuration
    PORT: int = int(os.getenv("PORT", 1440))
    WORKERS: int = int(os.getenv("WORKERS", 3))
    ADMIN_KEY: str = ""
    REGISTER_SECRET: str = os.getenv("REGISTER_SECRET", "")
    
    @field_validator('ADMIN_KEY', mode='before')
    @classmethod
    def validate_admin_key(cls, v):
        if v and v not in ("123456", "change-me-now", ""):
            return v
        
        key_file = DATA_DIR / ".admin_key"
        if key_file.exists():
            return key_file.read_text().strip()
        
        new_key = secrets.token_urlsafe(32)
        DATA_DIR.mkdir(parents=True, exist_ok=True)
        key_file.write_text(new_key)
        key_file.chmod(0o600)
        print(f"[QWENPI] Generated admin key: {new_key}")
        return new_key
    
    # MoeMail self-hosted configuration
    MOEMAIL_DOMAIN: str = os.getenv("MOEMAIL_DOMAIN", "")
    MOEMAIL_KEY: str = os.getenv("MOEMAIL_KEY", "")

    # TempMail (awsl.uk) self-hosted configuration
    TEMPMAIL_DOMAIN: str = os.getenv("TEMPMAIL_DOMAIN", "")
    TEMPMAIL_KEY: str = os.getenv("TEMPMAIL_KEY", "")

    # Engine mode: httpx (fast direct), browser (browser fingerprint, anti-ban) or hybrid (combined)
    ENGINE_MODE: str = os.getenv("ENGINE_MODE", "hybrid")
    NATIVE_TOOL_PASSTHROUGH: bool = os.getenv("NATIVE_TOOL_PASSTHROUGH", "true").lower() in ("1", "true", "yes", "on")
    # Browser engine configuration
    BROWSER_POOL_SIZE: int = int(os.getenv("BROWSER_POOL_SIZE", 2))
    MAX_INFLIGHT_PER_ACCOUNT: int = int(os.getenv("MAX_INFLIGHT", 1))
    STREAM_KEEPALIVE_INTERVAL: int = int(os.getenv("STREAM_KEEPALIVE_INTERVAL", 5))

    # Failover and rate limiting
    MAX_RETRIES: int = 3
    TOOL_MAX_RETRIES: int = 4
    EMPTY_RESPONSE_RETRIES: int = 1
    # Thinking mode: True=enabled by default (slower, higher quality), False=disabled by default (faster, may be slightly lower quality)
    DEFAULT_THINKING_ENABLED: bool = os.getenv("DEFAULT_THINKING_ENABLED", "false").lower() in ("1", "true", "yes", "on")
    ACCOUNT_MIN_INTERVAL_MS: int = int(os.getenv("ACCOUNT_MIN_INTERVAL_MS", 300))
    REQUEST_JITTER_MIN_MS: int = int(os.getenv("REQUEST_JITTER_MIN_MS", 30))
    REQUEST_JITTER_MAX_MS: int = int(os.getenv("REQUEST_JITTER_MAX_MS", 100))
    RATE_LIMIT_BASE_COOLDOWN: int = int(os.getenv("RATE_LIMIT_BASE_COOLDOWN", 600))
    RATE_LIMIT_MAX_COOLDOWN: int = int(os.getenv("RATE_LIMIT_MAX_COOLDOWN", 3600))
    RATE_LIMIT_COOLDOWN: int = RATE_LIMIT_BASE_COOLDOWN

    # AccountPool v2 — High-concurrency scheduling
    MAX_RPM_PER_ACCOUNT: int = int(os.getenv("MAX_RPM_PER_ACCOUNT", 50))
    MAX_TPM_PER_ACCOUNT: int = int(os.getenv("MAX_TPM_PER_ACCOUNT", 500000))
    CIRCUIT_BREAKER_THRESHOLD: int = int(os.getenv("CIRCUIT_BREAKER_THRESHOLD", 5))
    ACCOUNT_WARMUP_MINUTES: int = int(os.getenv("ACCOUNT_WARMUP_MINUTES", 120))

    # Auto-replenishment
    AUTO_REPLENISH: bool = os.getenv("AUTO_REPLENISH", "false").lower() in ("1", "true", "yes", "on")
    REPLENISH_TARGET: int = int(os.getenv("REPLENISH_TARGET", 30))
    REPLENISH_CONCURRENCY: int = int(os.getenv("REPLENISH_CONCURRENCY", 3))

    # Emergency replenishment on rate-limit exhaustion
    AUTO_REPLENISH_ON_EXHAUST: bool = os.getenv("AUTO_REPLENISH_ON_EXHAUST", "true").lower() in ("1", "true", "yes", "on")
    REPLENISH_EXHAUST_COUNT: int = int(os.getenv("REPLENISH_EXHAUST_COUNT", 10))
    REPLENISH_EXHAUST_CONCURRENCY: int = int(os.getenv("REPLENISH_EXHAUST_CONCURRENCY", 3))

    # Response cache
    CACHE_TTL_SECONDS: int = int(os.getenv("CACHE_TTL_SECONDS", 60))
    CACHE_MAX_SIZE: int = int(os.getenv("CACHE_MAX_SIZE", 500))

    # Racing mode
    RACING_ENABLED: bool = os.getenv("RACING_ENABLED", "false").lower() in ("1", "true", "yes", "on")

    # Registration proxy pool (used to bypass WAF rate limiting)
    PROXY_ENABLED: bool = os.getenv("PROXY_ENABLED", "false").lower() in ("1", "true", "yes", "on")
    PROXY_URL: str = os.getenv("PROXY_URL", "")              # e.g. http://host:port  socks5://host:port
    PROXY_USERNAME: str = os.getenv("PROXY_USERNAME", "")   # empty means no auth required
    PROXY_PASSWORD: str = os.getenv("PROXY_PASSWORD", "")   # empty means no auth required


    # Data file paths
    ACCOUNTS_FILE: str = os.getenv("ACCOUNTS_FILE", str(DATA_DIR / "accounts.json"))
    USERS_FILE: str = os.getenv("USERS_FILE", str(DATA_DIR / "users.json"))
    CAPTURES_FILE: str = os.getenv("CAPTURES_FILE", str(DATA_DIR / "captures.json"))
    CONFIG_FILE: str = os.getenv("CONFIG_FILE", str(DATA_DIR / "config.json"))

    class Config:
        env_file = ".env"
        extra = "ignore"

API_KEYS_FILE = DATA_DIR / "api_keys.json"

def load_api_keys() -> set:
    if API_KEYS_FILE.exists():
        try:
            with open(API_KEYS_FILE, "r", encoding="utf-8") as f:
                data = json.load(f)
                return set(data.get("keys", []))
        except Exception:
            pass
    return set()

def save_api_keys(keys: set):
    API_KEYS_FILE.parent.mkdir(parents=True, exist_ok=True)
    with open(API_KEYS_FILE, "w", encoding="utf-8") as f:
        json.dump({"keys": list(keys)}, f, indent=2)

# In-memory storage for managed API keys
API_KEYS = load_api_keys()

VERSION = "2.0.0"

settings = Settings()

# ── Runtime settings persistence (settings edited via the UI are saved to config.json) ──
_RUNTIME_CONFIG_FILE = DATA_DIR / "runtime_settings.json"
_PERSIST_KEYS = [
    "ADMIN_KEY",
    "AUTO_REPLENISH", "REPLENISH_TARGET", "REPLENISH_CONCURRENCY",
    "AUTO_REPLENISH_ON_EXHAUST", "REPLENISH_EXHAUST_COUNT", "REPLENISH_EXHAUST_CONCURRENCY",
    "MAX_INFLIGHT_PER_ACCOUNT", "MAX_RPM_PER_ACCOUNT", "MAX_TPM_PER_ACCOUNT",
    "CACHE_TTL_SECONDS", "RACING_ENABLED", "ENGINE_MODE",
    "MOEMAIL_DOMAIN", "MOEMAIL_KEY", "TEMPMAIL_DOMAIN", "TEMPMAIL_KEY",
    "PROXY_ENABLED", "PROXY_URL", "PROXY_USERNAME", "PROXY_PASSWORD",
]

def save_runtime_settings():
    """Persist UI-edited runtime settings to disk."""
    data = {}
    for key in _PERSIST_KEYS:
        val = getattr(settings, key, None)
        if val is not None:
            data[key] = val
    # MODEL_MAP is persisted separately
    data["MODEL_MAP"] = dict(MODEL_MAP)
    try:
        _RUNTIME_CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
        with open(_RUNTIME_CONFIG_FILE, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2, ensure_ascii=False)
    except Exception as e:
        logging.getLogger("qwenpi").warning(f"Failed to save runtime settings: {e}")

def _load_runtime_settings():
    """On startup, restore UI-edited runtime settings from disk."""
    if not _RUNTIME_CONFIG_FILE.exists():
        return
    try:
        with open(_RUNTIME_CONFIG_FILE, "r", encoding="utf-8") as f:
            data = json.load(f)
        for key, val in data.items():
            if key == "MODEL_MAP" and isinstance(val, dict):
                MODEL_MAP.clear()
                MODEL_MAP.update(val)
                continue
            if key in _PERSIST_KEYS and hasattr(settings, key):
                expected_type = type(getattr(settings, key))
                try:
                    setattr(settings, key, expected_type(val))
                except (ValueError, TypeError):
                    pass
        logging.getLogger("qwenpi").info(f"[Config] Restored {len(data)} runtime settings from runtime_settings.json")
    except Exception as e:
        logging.getLogger("qwenpi").warning(f"Failed to load runtime settings: {e}")

# ── Built-in models available on the Qwen website ──────────────────────────────────────────────────────
BUILTIN_MODELS = [
    "qwen3.6-plus",
    "qwen3.6-plus-thinking",
    "qwen3.6-plus-nothinking",
    "qwen3.6-max-preview",
    "qwen3.6-max-preview-thinking",
    "qwen3.6-max-preview-nothinking",
    "qwen3.6-27b",
    "qwen3.6-27b-thinking",
    "qwen3.6-27b-nothinking",
]

# Default model (fallback for unknown model names)
DEFAULT_MODEL = "qwen3.6-plus"

# Thinking-mode suffixes
NOTHINKING_SUFFIX = "-nothinking"
THINKING_SUFFIX = "-thinking"

# ── Default alias mapping (common OpenAI/Claude/Gemini model names → Qwen models) ──────────────
DEFAULT_MODEL_ALIASES: dict[str, str] = {
    # OpenAI family
    "gpt-4o": "qwen3.6-plus",
    "gpt-4o-mini": "qwen3.6-27b",
    "gpt-4-turbo": "qwen3.6-plus",
    "gpt-4": "qwen3.6-plus",
    "gpt-4.1": "qwen3.6-plus",
    "gpt-4.1-mini": "qwen3.6-27b",
    "gpt-4.1-nano": "qwen3.6-27b",
    "gpt-3.5-turbo": "qwen3.6-27b",
    "gpt-3.5": "qwen3.6-27b",
    "o1": "qwen3.6-max-preview",
    "o1-mini": "qwen3.6-plus",
    "o1-preview": "qwen3.6-max-preview",
    "o3": "qwen3.6-max-preview",
    "o3-mini": "qwen3.6-plus",
    "o4-mini": "qwen3.6-plus",
    # Claude family
    "claude-3-5-sonnet-latest": "qwen3.6-max-preview",
    "claude-3-5-sonnet-20241022": "qwen3.6-max-preview",
    "claude-3-5-haiku-latest": "qwen3.6-plus",
    "claude-3-opus-latest": "qwen3.6-max-preview",
    "claude-3-sonnet-20240229": "qwen3.6-plus",
    "claude-3-haiku-20240307": "qwen3.6-27b",
    "claude-sonnet-4-20250514": "qwen3.6-max-preview",
    "claude-haiku-4-20250514": "qwen3.6-plus",
    # Gemini family
    "gemini-2.5-pro": "qwen3.6-max-preview",
    "gemini-2.5-flash": "qwen3.6-plus",
    "gemini-2.0-flash": "qwen3.6-plus",
    "gemini-1.5-pro": "qwen3.6-plus",
    "gemini-1.5-flash": "qwen3.6-27b",
    "gemini-pro": "qwen3.6-plus",
    # DeepSeek family
    "deepseek-chat": "qwen3.6-plus",
    "deepseek-reasoner": "qwen3.6-max-preview",
    "deepseek-coder": "qwen3.6-plus",
    # Qwen legacy names / aliases
    "qwen-plus": "qwen3.6-plus",
    "qwen-max": "qwen3.6-max-preview",
    "qwen-turbo": "qwen3.6-27b",
    "qwen-long": "qwen3.6-plus",
    "qwen-coder-plus": "qwen3.6-plus",
    "qwq-plus": "qwen3.6-max-preview",
    "qwq-max": "qwen3.6-max-preview",
}

# User-defined mapping (configured from admin UI, highest priority)
MODEL_MAP: dict = {}

# Restore from the persisted file at startup (includes MODEL_MAP)
_load_runtime_settings()

# Image generation reuses the base model that the website is currently serving
IMAGE_MODEL_DEFAULT = "qwen3.6-plus"


def resolve_model(name: str) -> str:
    """Resolve a model name to the real Qwen model name.

    The -nothinking / -thinking suffix is stripped before being sent upstream;
    thinking mode is determined separately by resolve_model_thinking().
    """
    # 1. User-defined mapping (highest priority)
    if name in MODEL_MAP:
        resolved = MODEL_MAP[name]
        if resolved.endswith(NOTHINKING_SUFFIX):
            return resolved[:-len(NOTHINKING_SUFFIX)]
        if resolved.endswith(THINKING_SUFFIX):
            return resolved[:-len(THINKING_SUFFIX)]
        return resolved
    # 2. Strip suffixes and look up again
    base_name = name
    if name.endswith(NOTHINKING_SUFFIX):
        base_name = name[:-len(NOTHINKING_SUFFIX)]
    elif name.endswith(THINKING_SUFFIX):
        base_name = name[:-len(THINKING_SUFFIX)]
    # 3. Default alias mapping
    if base_name in DEFAULT_MODEL_ALIASES:
        return DEFAULT_MODEL_ALIASES[base_name]
    # 4. Already a built-in real model name
    real_models = {"qwen3.6-plus", "qwen3.6-max-preview", "qwen3.6-27b"}
    if base_name in real_models:
        return base_name
    # 5. Unknown — fall back to default
    return DEFAULT_MODEL


def resolve_model_thinking(name: str) -> bool | None:
    """Determine the thinking mode from the model name.

    -nothinking suffix = False (fast mode, thinking off)
    -thinking suffix = True (forced thinking mode)
    no suffix = None (auto, decided by Qwen)
    """
    if name in MODEL_MAP:
        resolved = MODEL_MAP[name]
        if resolved.endswith(NOTHINKING_SUFFIX):
            return False
        if resolved.endswith(THINKING_SUFFIX):
            return True
    if name.endswith(NOTHINKING_SUFFIX):
        return False
    if name.endswith(THINKING_SUFFIX):
        return True
    return None  # auto mode


def get_all_available_models() -> list[str]:
    """Return all available model names (built-in + default aliases + user-defined)."""
    all_names = set(BUILTIN_MODELS)
    all_names.update(DEFAULT_MODEL_ALIASES.keys())
    all_names.update(MODEL_MAP.keys())
    all_names.update(MODEL_MAP.values())
    return sorted(all_names)


def validate_security_config():
    """Validate security-related configuration on startup."""
    _log = logging.getLogger("qwenpi.config")
    if not settings.ADMIN_KEY:
        _log.warning("[Security] ADMIN_KEY is not configured; using default value 123456")
        settings.ADMIN_KEY = "123456"
