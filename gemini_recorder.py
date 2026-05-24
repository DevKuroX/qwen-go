#!/home/ubuntu/scripts/auth/.venv/bin/python3
"""
Gemini Cookie Recorder + API Test Script
Opens Camoufox on :20 (CRD visible), waits for login, captures cookies, tests API.

Usage:
    /home/ubuntu/scripts/auth/.venv/bin/python3 /home/ubuntu/scripts/gemini_recorder.py

Steps:
    1. Opens Camoufox to gemini.google.com (visible on CRD :20)
    2. Waits for user to log in via CRD
    3. Saves cookies to /tmp/gemini_cookies.json
    4. Auto-closes browser after login detected
    5. Tests Gemini-API with captured cookies
"""

import json
import os
import sys
import time
from pathlib import Path

PROXY_URL = "http://priv8idio2:ssh19233@198.23.147.126:5141"
GEMINI_URL = "https://gemini.google.com/"
COOKIE_FILE = "/tmp/gemini_cookies.json"
CRD_DISPLAY = ":20"
REQUIRED_COOKIES = ["__Secure-1PSID", "__Secure-1PSIDTS"]


def save_cookies(cookies_dict):
    with open(COOKIE_FILE, "w") as f:
        json.dump(cookies_dict, f, indent=2)
    print(f"\nCookies saved to {COOKIE_FILE}")
    for name in REQUIRED_COOKIES:
        val = cookies_dict.get(name, "")
        print(f"    {name}: {val[:30]}..." if val else f"    {name}: MISSING!")


def is_logged_in(cookies):
    for name in REQUIRED_COOKIES:
        val = cookies.get(name)
        if not val or val == "" or len(val) < 10:
            return False
    return True


def wait_for_login(page, check_interval=2, timeout_minutes=30):
    """Wait for user to log in via browser on CRD."""
    print("\nWaiting for Gemini login...")
    print("    Open CRD at https://remotedesktop.google.com/access")
    print("    Or VNC to localhost:5900")
    print("    Navigate to gemini.google.com and log in with your Google account.")
    print(f"    Timeout: {timeout_minutes} minutes")

    start = time.time()
    last_url = ""
    while time.time() - start < timeout_minutes * 60:
        try:
            cookies = {}
            all_cookies = page.context.cookies()
            for c in all_cookies:
                if c["name"] in REQUIRED_COOKIES:
                    cookies[c["name"]] = c["value"]

            if is_logged_in(cookies):
                elapsed = time.time() - start
                print(f"\nLogin detected! ({elapsed:.0f}s elapsed)")
                return cookies

            current_url = page.url
            if current_url != last_url:
                print(f"  URL: {current_url[:80]}")
                last_url = current_url
        except Exception as e:
            pass

        time.sleep(check_interval)

    print(f"\nTimeout after {timeout_minutes} minutes.")
    return None


import asyncio

async def test_gemini_api(cookies):
    """Test the Gemini-API with captured cookies."""
    print("\nTesting Gemini API...")
    from gemini_webapi import GeminiClient

    client = GeminiClient(
        secure_1psid=cookies["__Secure-1PSID"],
        secure_1psidts=cookies["__Secure-1PSIDTS"],
        proxy=PROXY_URL,
    )

    print("  Initializing client...")
    try:
        await client.init()
        print("  Init: OK")
    except Exception as e:
        print(f"  Init FAILED: {e}")
        return

    print("  Testing basic chat...")
    try:
        resp = await client.generate_content("Hello! What is 2+2? Answer briefly.")
        print(f"  Response: {resp.text}")
        print("  Basic chat: OK")
    except Exception as e:
        print(f"  Basic chat FAILED: {e}")

    print("  Testing model list...")
    try:
        models = client.list_models()
        if models:
            for m in models:
                print(f"    - {m}")
        else:
            print(f"    (none returned)")
        print("  Model list: OK")
    except Exception as e:
        print(f"  Model list FAILED: {e}")


def main():
    print("=" * 60)
    print("  Gemini Cookie Recorder + API Test")
    print("=" * 60)

    print(f"  Display: {CRD_DISPLAY}")
    print(f"  Proxy: {PROXY_URL.split('@')[1] if '@' in PROXY_URL else PROXY_URL}")
    print(f"  Cookie file: {COOKIE_FILE}")
    print(f"  Python: {sys.executable}")
    print()

    from camoufox import Camoufox

    print("Opening Camoufox...")

    with Camoufox(
        headless=False,
        geoip=True,
        proxy={"server": PROXY_URL},
        screen={"width": 1280, "height": 800, "color_depth": 24},
        humanize=False,
    ) as browser:
        context = browser.new_context(
            viewport={"width": 1280, "height": 800}
        )
        page = context.new_page()

        print(f"Navigating to {GEMINI_URL}...")
        page.goto(GEMINI_URL, wait_until="domcontentloaded", timeout=30000)

        cookies = wait_for_login(page)

        if cookies and is_logged_in(cookies):
            save_cookies(cookies)
            asyncio.run(test_gemini_api(cookies))
        else:
            print("Login not detected or incomplete.")

        print("Auto-closing browser...")

    print()
    if cookies and is_logged_in(cookies):
        print("RESULT: SUCCESS - Cookies captured and API tested")
        print(f"  Cookie file: {COOKIE_FILE}")
    else:
        print("RESULT: FAILED - No valid cookies obtained")

    print("=" * 60)


if __name__ == "__main__":
    main()
