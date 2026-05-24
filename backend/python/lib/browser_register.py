"""
browser_register.py — Headless browser registration (captcha-free pass-through strategy)

Strategy:
  - Rotate IPs through a proxy pool, aiming for "captcha-free" clean IPs for registration
  - After clicking "Create account", wait 5s:
      • No captcha appears → success, return cookies
      • Captcha appears   → discard immediately (do not solve), return None
  - Speed: ~7s per attempt  |  Memory: no cv2 / numpy

The caller (register.py) handles concurrent retries; proxy rotation is performed automatically by the rotating proxy URL.
"""

import asyncio
import logging
import random
from typing import Optional

log = logging.getLogger("qwenpi.browser_register")


def _translate_err(e: Exception) -> str:
    """Translate Playwright exception messages into user-friendly English."""
    msg = str(e)
    if "Locator.fill" in msg and "Timeout" in msg:
        return "Form fill timeout (registration page loaded too slowly or the element did not appear)"
    if "Locator.click" in msg and "Timeout" in msg:
        return "Button click timeout (page not fully loaded)"
    if "Timeout" in msg and ("goto" in msg or "navigate" in msg or "networkidle" in msg):
        return "Page load timeout (slow network or unstable proxy)"
    if "net::ERR_" in msg:
        code = msg.split("net::ERR_")[-1].split()[0].rstrip(")")
        return f"Network error (ERR_{code}, please check the proxy connection)"
    if "Browser" in msg and ("closed" in msg or "crash" in msg):
        return "Browser closed unexpectedly"
    if "Timeout" in msg:
        return f"Operation timed out ({msg[:60]})"
    return msg[:120]


# ────────────────────────────────────────────────────────────────
# Core: headless browser registration (captcha-free pass-through)
# ────────────────────────────────────────────────────────────────

async def browser_signup(
    email: str,
    password_hash: str,
    password_plain: str = "AlIlzHZkJ4zG6J",
) -> Optional[dict]:
    """
    Submit the Qwen registration form via Playwright.

    - Proxy pool config is read from settings on each call (rotating proxy supported)
    - Returns None immediately when a captcha appears, leaving retries to the caller
    - On success, returns {"success": True, "cookies": {...}}
    """
    from playwright.async_api import async_playwright
    from lib.config import settings as _cfg  # hot reload

    log.info(f"[Register] [{email}] Starting registration")

    # ── Proxy pool config (supports inline auth: http://user:pass@host:port) ──
    _proxy: dict | None = None
    if getattr(_cfg, "PROXY_ENABLED", False) and getattr(_cfg, "PROXY_URL", ""):
        from urllib.parse import urlparse, urlunparse
        raw = _cfg.PROXY_URL
        parsed = urlparse(raw)
        username = getattr(_cfg, "PROXY_USERNAME", "") or parsed.username or ""
        password = getattr(_cfg, "PROXY_PASSWORD", "") or parsed.password or ""
        # Strip embedded auth from URL (Playwright requires username/password to be passed separately)
        clean = parsed._replace(netloc=parsed.hostname + (f":{parsed.port}" if parsed.port else ""))
        server = urlunparse(clean)
        _proxy = {"server": server}
        if username:
            _proxy["username"] = username
        if password:
            _proxy["password"] = password
        log.info(f"[Register] [{email}] Using proxy: {server}")
    else:
        log.info(f"[Register] [{email}] Direct connection mode (proxy disabled)")

    try:
        async with async_playwright() as pw:
            browser = await pw.chromium.launch(
                headless=True,
                proxy=_proxy,
                args=[
                    "--disable-blink-features=AutomationControlled",
                    "--no-sandbox",
                    "--disable-dev-shm-usage",
                    "--disable-gpu",
                ],
            )
            ctx_kwargs = dict(
                viewport={"width": 1280, "height": 800},
                user_agent=(
                    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) "
                    "Chrome/131.0.0.0 Safari/537.36"
                ),
                locale="zh-CN",
            )
            if _proxy:
                ctx_kwargs["proxy"] = _proxy
            context = await browser.new_context(**ctx_kwargs)

            # Anti webdriver detection
            await context.add_init_script(
                "Object.defineProperty(navigator, 'webdriver', { get: () => undefined });"
            )
            page = await context.new_page()

            # ── 1. Open the registration page ──
            log.info(f"[Register] [{email}] Opening registration page...")
            try:
                await page.goto(
                    "https://chat.qwen.ai/auth?mode=register",
                    wait_until="networkidle",
                    timeout=30000,
                )
            except Exception:
                await page.goto(
                    "https://chat.qwen.ai/auth?mode=register",
                    wait_until="domcontentloaded",
                    timeout=30000,
                )
            await page.wait_for_timeout(random.randint(800, 1200))

            # ── 2. Fill in the form ──
            log.info(f"[Register] [{email}] Filling registration form...")
            random_name = f"user{''.join([str(random.randint(0, 9)) for _ in range(6)])}"
            try:
                # Placeholder text on the upstream page is the Chinese word for "name"
                await page.locator('input[placeholder*="\u540d\u79f0"]').fill(random_name)
            except Exception:
                pass
            await page.wait_for_timeout(random.randint(200, 400))

            # Placeholder text on the upstream page is the Chinese word for "email"
            await page.locator('input[placeholder*="\u90ae\u7bb1"]').fill(email)
            await page.wait_for_timeout(random.randint(200, 400))

            password_inputs = page.locator('input[type="password"]')
            count = await password_inputs.count()
            if count >= 2:
                await password_inputs.nth(0).fill(password_plain)
                await page.wait_for_timeout(random.randint(150, 300))
                await password_inputs.nth(1).fill(password_plain)
            elif count == 1:
                await password_inputs.nth(0).fill(password_plain)
            await page.wait_for_timeout(random.randint(200, 350))

            checkbox = page.locator('input[type="checkbox"]')
            if await checkbox.count() > 0:
                await checkbox.first.click()
                await page.wait_for_timeout(200)

            # ── 3. Click "Create account" ──
            log.info(f"[Register] [{email}] Clicking Create account...")
            # The button text on the upstream page is the Chinese phrase for "Create account"
            await page.locator('button:has-text("\u521b\u5efa\u8d26\u53f7")').click()

            # ── 4. Wait 5s, then determine the result ──
            # Captcha selectors (Aliyun WAF slider)
            CAPTCHA_SELS = [
                "#waf_nc_block",
                "#WAF_NC_WRAPPER",
                ".waf-nc-wrapper",
                "#aliyunCaptcha-sliding-slider",
                '[class*="nc-wrapper"]',
                '[class*="captcha"]',
            ]
            captcha_loc = page.locator(", ".join(CAPTCHA_SELS))

            # Wait up to 5s to see whether a captcha appears
            captcha_appeared = False
            try:
                await captcha_loc.first.wait_for(state="visible", timeout=5000)
                captcha_appeared = True
            except Exception:
                pass  # timeout → no captcha appeared → good

            if captcha_appeared:
                log.warning(f"[Register] [{email}] ⚠️ Captcha required, discarding this email")
                await browser.close()
                return None

            # No captcha → registration succeeded
            log.info(f"[Register] [{email}] ✅ No captcha, waiting for activation email")
            await page.wait_for_timeout(2000)  # Wait for server-side processing

            cookies_list = await context.cookies()
            cookies_dict = {c["name"]: c["value"] for c in cookies_list}
            log.info(f"[Register] [{email}] Captured {len(cookies_dict)} cookies")
            await browser.close()
            return {"success": True, "cookies": cookies_dict}

    except Exception as e:
        log.error(f"[Register] [{email}] Registration process error: {_translate_err(e)}")
        return None


def browser_signup_sync(
    email: str,
    password_hash: str,
    password_plain: str = "AlIlzHZkJ4zG6J",
) -> Optional[dict]:
    """
    Synchronous wrapper for use in a thread pool.
    Runs the async browser_signup on a fresh event loop.
    """
    try:
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        result = loop.run_until_complete(browser_signup(email, password_hash, password_plain))
        loop.close()
        return result
    except Exception as e:
        log.error(f"[BrowserRegister] Sync wrapper exception: {e}")
        return None
