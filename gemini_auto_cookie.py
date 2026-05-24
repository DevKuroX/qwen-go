#!/home/ubuntu/scripts/auth/.venv/bin/python3
"""
Gemini Auto Cookie Capture — Playwright/Chrome + Camoufox fallback.
Auto-detects login state, captures cookies, tests via API, saves to DB.

Usage:
  # Interactive: open browser on CRD display :20, wait for manual login
  /home/ubuntu/scripts/auth/.venv/bin/python3 gemini_auto_cookie.py

  # Batch: process accounts from file (email:password per line)
  /home/ubuntu/scripts/auth/.venv/bin/python3 gemini_auto_cookie.py --accounts /path/to/accounts.txt

  # Quick: test existing cookies in DB, recap if expired
  /home/ubuntu/scripts/auth/.venv/bin/python3 gemini_auto_cookie.py --refresh
"""

import argparse
import asyncio
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

# Ensure auth/ package is importable
_SCRIPT_DIR = Path(__file__).resolve().parent
_AUTH_DIR = str(_SCRIPT_DIR / "auth")
if _AUTH_DIR not in sys.path:
    sys.path.insert(0, _AUTH_DIR)

# --- Configuration ---
PROXY = os.getenv("GEMINI_PROXY", "")
GEMINI_URL = "https://gemini.google.com/"
COOKIE_FILE = "/tmp/gemini_cookies.json"
REQUIRED_COOKIES = ["__Secure-1PSID", "__Secure-1PSIDTS"]
CRD_DISPLAY = os.getenv("CRD_DISPLAY", ":20")
DB_DSN = os.getenv("DB_DSN", "postgresql://aiproxy:aiproxy123@localhost:5432/aiproxy?sslmode=disable")
PROVIDER_ID = "3"  # gemini-web in providers table
VENV_PYTHON = "/home/ubuntu/scripts/auth/.venv/bin/python3"

# --- Helpers ---


def log(msg: str):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def save_cookies(cookies: dict, path: str = COOKIE_FILE):
    with open(path, "w") as f:
        json.dump(cookies, f, indent=2)
    log(f"Cookies saved to {path}")
    for name in REQUIRED_COOKIES:
        val = cookies.get(name, "")
        print(f"    {name}: {val[:40]}..." if val else f"    {name}: MISSING!")


def is_logged_in(cookies: dict) -> bool:
    for name in REQUIRED_COOKIES:
        val = cookies.get(name)
        if not val or len(val) < 20:
            return False
    return True


async def test_cookies(cookies: dict) -> bool:
    """Test cookies via gemini_webapi."""
    log("Testing cookies via Gemini API...")
    try:
        from gemini_webapi import GeminiClient

        client = GeminiClient(
            secure_1psid=cookies["__Secure-1PSID"],
            secure_1psidts=cookies["__Secure-1PSIDTS"],
            proxy=PROXY,
        )
        await client.init()
        log("Init: OK")
        resp = await client.generate_content("Reply with just: OK")
        log(f"API test: {resp.text}")
        return True
    except Exception as e:
        log(f"API test FAILED: {e}")
        return False


def save_to_db(cookies: dict, name: str = "gemini-auto"):
    """Save cookies to provider_accounts table via psql."""
    psid = cookies["__Secure-1PSID"].replace("'", "''")
    psidts = cookies["__Secure-1PSIDTS"].replace("'", "''")
    name_esc = name.replace("'", "''")
    sql = (
        f"INSERT INTO provider_accounts (provider_id, name, api_key, refresh_token, is_active) "
        f"SELECT '{PROVIDER_ID}', '{name_esc}', '{psid}', '{psidts}', true "
        f"WHERE NOT EXISTS (SELECT 1 FROM provider_accounts WHERE provider_id = '{PROVIDER_ID}' AND name = '{name_esc}')"
    )
    try:
        subprocess.run(
            ["psql", DB_DSN, "-c", sql],
            capture_output=True, text=True, timeout=10,
        )
        log(f"Cookies saved to DB (provider_accounts, name={name})")
    except Exception as e:
        log(f"DB save FAILED: {e}")


# --- Browser Methods ---


def extract_cookies_from_context(context) -> dict:
    """Extract Gemini cookies from Playwright/Camoufox context."""
    all_cookies = context.cookies()
    result = {}
    domain_counts = {}
    for c in all_cookies:
        d = c.get("domain", "")
        domain_counts[d] = domain_counts.get(d, 0) + 1
    log(f"Cookies by domain: {dict(sorted(domain_counts.items(), key=lambda x: -x[1])[:10])}")
    for c in all_cookies:
        if c["name"] in REQUIRED_COOKIES:
            result[c["name"]] = c["value"]
    return result


async def extract_cookies_async(context) -> dict:
    """Extract Gemini cookies from async Camoufox context."""
    all_cookies = await context.cookies()
    result = {}
    for c in all_cookies:
        if c["name"] in REQUIRED_COOKIES:
            result[c["name"]] = c["value"]
    return result


async def capture_playwright(display: str) -> dict:
    """Capture cookies using Playwright + Chromium (stealth)."""
    log("Method 1: Playwright + Chromium...")
    os.environ["DISPLAY"] = display

    from playwright.async_api import async_playwright

    async with async_playwright() as pw:
        browser = await pw.chromium.launch(
            headless=False,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-dev-shm-usage",
                f"--display={display}",
            ],
        )
        context = await browser.new_context(
            viewport={"width": 1280, "height": 800},
            user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
        )

        # Anti-detection: override navigator.webdriver
        await context.add_init_script("""
            Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
            Object.defineProperty(navigator, 'plugins', { get: () => [1,2,3,4,5] });
            window.chrome = { runtime: {} };
        """)

        page = await context.new_page()
        log(f"Navigating to {GEMINI_URL}...")
        await page.goto(GEMINI_URL, wait_until="domcontentloaded", timeout=30000)

        # Wait and check login state
        cookies = await _wait_for_login(page, context, "Playwright")

        await browser.close()
        return cookies


async def capture_camoufox(display: str) -> dict:
    """Capture cookies using Camoufox (stealth Firefox) as fallback."""
    log("Method 2: Camoufox (stealth Firefox)...")
    os.environ["DISPLAY"] = display

    from camoufox import Camoufox

    is_headless = os.getenv("HEADLESS", "true").lower() == "true"
    kwargs = dict(
        headless=is_headless,
        geoip=False,
        screen={"width": 1280, "height": 800, "color_depth": 24},
        humanize=False,
    )
    if PROXY:
        kwargs["proxy"] = {"server": PROXY}
    log(f"Camoufox headless={is_headless} display={os.getenv('DISPLAY', 'none')}")

    with Camoufox(**kwargs) as browser:
        context = browser.new_context(viewport={"width": 1280, "height": 800})
        page = context.new_page()
        log(f"Navigating to {GEMINI_URL}...")
        await page.goto(GEMINI_URL, wait_until="domcontentloaded", timeout=30000)

        cookies = await _wait_for_login(page, context, "Camoufox")
        return cookies


async def _wait_for_login(page, context, method_name: str, timeout_minutes: int = 30) -> dict:
    """Wait for user to log in via visible browser, checking cookies periodically."""
    log(f"   Open CRD at https://remotedesktop.google.com/access (display {CRD_DISPLAY})")
    log(f"   Navigate to gemini.google.com and log in with Google account")
    log(f"   Timeout: {timeout_minutes} minutes")

    # First check if already logged in
    cookies = extract_cookies_from_context(context)
    if is_logged_in(cookies):
        log(f"{method_name}: Already logged in!")
        return cookies

    start = time.time()
    last_url = ""
    while time.time() - start < timeout_minutes * 60:
        try:
            cookies = extract_cookies_from_context(context)
            if is_logged_in(cookies):
                elapsed = time.time() - start
                log(f"{method_name}: Login detected! ({elapsed:.0f}s elapsed)")
                return cookies

            try:
                current_url = page.url
                if current_url != last_url:
                    print(f"  URL: {current_url[:100]}")
                    last_url = current_url
            except Exception:
                pass

            # Check for captcha or blocked
            body_text = await page.content()
            if "unusual traffic" in body_text.lower() or "captcha" in body_text.lower():
                log(f"WARNING: {method_name} detected as bot! Switching method...")
                return {"error": "blocked"}

        except Exception as e:
            pass

        await asyncio.sleep(2)

    log(f"Timeout after {timeout_minutes} minutes.")
    return {}


async def auto_capture_cookies(display: str = CRD_DISPLAY) -> dict:
    """Try Playwright first, fallback to Camoufox on failure."""
    cookies = {}
    errors = []

    # Method 1: Playwright + Chromium
    try:
        cookies = await capture_playwright(display)
        if cookies.get("error") == "blocked":
            log("Playwright blocked — switching to Camoufox")
            errors.append("playwright_blocked")
            cookies = {}
    except Exception as e:
        log(f"Playwright failed: {e}")
        errors.append(f"playwright_error:{e}")
        cookies = {}

    # Method 2: Camoufox fallback
    if not cookies or not is_logged_in(cookies):
        try:
            cookies = await capture_camoufox(display)
        except Exception as e:
            log(f"Camoufox also failed: {e}")
            errors.append(f"camoufox_error:{e}")

    return cookies, errors


# --- Batch Account Processing ---


async def process_accounts_file(path: str):
    """Process email:password accounts using AsyncCamoufox + Google login helpers."""
    log(f"Processing accounts from {path}")

    from camoufox.async_api import AsyncCamoufox
    from app.providers.kiro import (
        _fill_google_email_step,
        _fill_google_password_step,
        _handle_google_gaplustos,
        _handle_google_consent_continue,
        _is_email_step,
        _is_password_step,
    )

    with open(path) as f:
        accounts = [line.strip() for line in f if line.strip() and ":" in line]

    log(f"Loaded {len(accounts)} accounts")
    success = 0

    for i, line in enumerate(accounts):
        email, password = line.split(":", 1)
        log(f"[{i + 1}/{len(accounts)}] Processing {email}...")

        try:
            is_headless = os.getenv("HEADLESS", "true").lower() == "true"
            kwargs = dict(
                headless=is_headless,
                geoip=False,
                screen={"width": 1280, "height": 800},
                humanize=True,
            )
            if PROXY:
                kwargs["proxy"] = {"server": PROXY}

            async with AsyncCamoufox(**kwargs) as browser:
                context = await browser.new_context()
                page = await context.new_page()
                await page.goto(GEMINI_URL, wait_until="domcontentloaded", timeout=30000)

                for attempt in range(60):
                    current_url = page.url
                    cookies = await extract_cookies_async(context)

                    # Check if we got Gemini cookies
                    if is_logged_in(cookies):
                        log(f"  Login OK for {email}")
                        valid = await test_cookies(cookies)
                        if valid:
                            safe_name = email.split("@")[0].replace(".", "_")
                            save_to_db(cookies, safe_name)
                            save_cookies(cookies, f"/tmp/gemini_cookies_{safe_name}.json")
                            success += 1
                            break

                    # If on Gemini but not logged in, click Sign in
                    if "gemini.google.com" in current_url and "signin" not in current_url.lower():
                        try:
                            has_login_btn = await page.evaluate(
                                """() => {
                                    const btns = document.querySelectorAll('a[href*="accounts.google.com"], a[href*="signin"], a[href*="login"], a[href*="ServiceLogin"]');
                                    for (const b of btns) {
                                        if (b.offsetParent !== null) {
                                            b.click();
                                            return true;
                                        }
                                    }
                                    return false;
                                }"""
                            )
                            if has_login_btn:
                                log("  Clicked Sign in button")
                                await asyncio.sleep(2)
                                continue
                        except Exception:
                            pass

                    # Handle Google login steps
                    if "accounts.google.com" in current_url:
                        if await _is_email_step(page):
                            log("  Filling email...")
                            await _fill_google_email_step(page, email)
                        elif await _is_password_step(page):
                            log("  Filling password...")
                            await _fill_google_password_step(page, password)
                        elif await _handle_google_consent_continue(page):
                            log("  Handling consent...")
                        elif await _handle_google_gaplustos(page):
                            log("  Handling G+ TOS...")
                        else:
                            log(f"  Google login page: {current_url[:80]}")
                    elif "gemini.google.com" in current_url:
                        if attempt % 10 == 0:
                            log("  On Gemini, waiting for login...")
                    else:
                        if attempt % 10 == 0:
                            log(f"  Current URL: {current_url[:80]}")

                    await asyncio.sleep(2)
                else:
                    log(f"  Login timeout for {email}")
                    cookies = await extract_cookies_async(context)
                    if is_logged_in(cookies):
                        save_cookies(cookies)
                        log("  Partial cookies saved")

        except Exception as e:
            log(f"  Failed for {email}: {e}")
            import traceback
            traceback.print_exc()

    log(f"Batch complete: {success}/{len(accounts)} success")


# --- Refresh Mode ---


async def refresh_mode():
    """Test existing DB accounts via psql, re-capture expired ones."""
    result = subprocess.run(
        ["psql", DB_DSN, "-t", "-A",
         "-c", f"SELECT id, name, api_key, refresh_token FROM provider_accounts WHERE provider_id = '{PROVIDER_ID}' AND is_active = true"],
        capture_output=True, text=True, timeout=10,
    )
    lines = [l.strip() for l in result.stdout.split("\n") if l.strip()]
    accounts = []
    for line in lines:
        parts = line.split("|")
        if len(parts) >= 4:
            accounts.append((parts[0], parts[1], parts[2], parts[3]))

    log(f"Found {len(accounts)} active gemini-web accounts in DB")
    refresh_needed = 0

    for acc_id, name, api_key, refresh_token in accounts:
        cookies = {"__Secure-1PSID": api_key, "__Secure-1PSIDTS": refresh_token}
        log(f"  Testing {name} (id={acc_id})...")
        valid = await test_cookies(cookies)
        if valid:
            log(f"  {name}: OK")
        else:
            log(f"  {name}: EXPIRED — needs recapture")
            refresh_needed += 1

    if refresh_needed > 0:
        log(f"\n{refresh_needed} account(s) expired. Opening browser for recapture...")
        cookies, _ = await auto_capture_cookies()
        if cookies and is_logged_in(cookies):
            log("  New cookies captured. Testing...")
            valid = await test_cookies(cookies)
            if valid:
                save_to_db(cookies, f"gemini-refreshed-{int(time.time())}")
    else:
        log("All accounts valid. No refresh needed.")


# --- Main ---


def main():
    parser = argparse.ArgumentParser(description="Gemini Auto Cookie Capture")
    parser.add_argument("--accounts", "-a", help="Path to accounts file (email:password per line)")
    parser.add_argument("--refresh", "-r", action="store_true", help="Test existing DB cookies, refresh if expired")
    parser.add_argument("--display", "-d", default=CRD_DISPLAY, help=f"X display (default: {CRD_DISPLAY})")
    parser.add_argument("--no-camoufox", action="store_true", help="Skip Camoufox fallback, only use Playwright")
    parser.add_argument("--headless", action="store_true", default=os.getenv("HEADLESS", "true").lower() == "true",
                        help="Run browser headless (default: true on VPS)")
    args = parser.parse_args()

    os.environ["DISPLAY"] = args.display

    if args.refresh:
        asyncio.run(refresh_mode())
        return

    if args.accounts:
        asyncio.run(process_accounts_file(args.accounts))
        return

    # Default: interactive capture
    log("=" * 60)
    log("  Gemini Auto Cookie Capture")
    log("=" * 60)
    log(f"  Display: {args.display}")
    log(f"  Proxy: {PROXY.split('@')[1] if '@' in PROXY else PROXY}")
    log(f"  Python: {sys.executable}")
    log()

    cookies, errors = asyncio.run(auto_capture_cookies(args.display))

    if cookies and is_logged_in(cookies):
        save_cookies(cookies)
        log("\nTesting captured cookies...")
        valid = asyncio.run(test_cookies(cookies))
        if valid:
            save_to_db(cookies)
            log("\nRESULT: SUCCESS")
        else:
            log("\nRESULT: Cookies captured but API test FAILED")
    else:
        log("\nRESULT: FAILED — no valid cookies obtained")
        if errors:
            log(f"Errors: {errors}")

    log("Auto-closing browser...")


if __name__ == "__main__":
    main()
