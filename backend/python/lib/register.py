"""
register.py — Bulk registration service
Wraps the registration logic from the root main.py as async functions for use by the admin API.
"""

import asyncio
import base64
import hashlib
import json
import logging
import os
import secrets
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Optional

log = logging.getLogger("qwenpi.register")

# Qwen registration endpoint
SIGNUP_URL = "https://chat.qwen.ai/api/v1/auths/signup"

# Default config used during registration
DEFAULT_PASSWORD_HASH = "3e44fb4816bed138eb46440954b79b3518d6cde7a58248d770410cb6be563c89"
DEFAULT_PROFILE_IMAGE = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAYAAABw4pVUAAAAAXNSR0IArs4c6QAAAlxJREFUeF7tmbFKHGEURu+4O3FX0AdICq2NETWddgERQQSx3ULfIYVFHiBFXk/fQDsLsZSQsFokCopX5o5n4Ww99/7fnjPfDMs2p5PrP+EHQ6BRCMbFfRCFsHwoBOZDIQqhEYDl8R2iEBgBWBwbohAYAVgcG6IQGAFYHBuiEBgBWBwbohAYAVgcWyIQGAFYHBuiEBgBWBwbohAYAVgcWyIQmAFYHBuiEBgBWBwbohAYAVgcG6IQGAFYHBuiEBgBWBwbohAYAVgcWyIQmAFYHBuiEBgBWBwbopDuCIxGTezujeJb7nwsLc1F00RcXf6OH2c33R3S86aZa8jHToPY2GpjbbWN5ZVhjBeaR8gU0uMddHg0joHDcfzxUoX0KOSY+P7BWPYOFNIj8peP2tz6EJtf25wbJO+RL+tttO3Do8uGvLOu7Z35mJwsxFSMQt5ZxvR4hQAk/B9BIQopJTBzv0Oe0rAhpfdHfrlC8sxKJxRSije/XCF5ZqUTCinFm1+ukDyz0gmFlOLNL1dInlnphEJK8eaXKyTPrHRCIaV488sVkmdWOqGQUrz55QrJMyudUEgp3vxyheSZlU4opBRvfrlC8sw6mfh+firn9s277o4v4tfP2/fPN/X4Mz8hauQvm6JV56jkFeC8rJuCczMI6vbr83dphCYG4UoBEYAFseGKARGABbHhigERgAWx4YoBEYAFseGKARGABbHhigERgAWx4YoBEYAFseGKARGABbHhigERgAWx4YoBEYAFseGKARGABbHhigERgAWx4YoBEYAFseGKARGABbHhigERgAWx4YoBEYAFseGKARGABbHhigERgAWx4YoBEYAFucv1Ia+eKkOMMMAAAAASUVORK5CYII="

# MailService provider configuration
MAIL_PROVIDERS = {
    "default": {"api_url": "https://mail.chatgpt.org.uk"},
    "guerrilla": {"api_url": None},  # Use the official GuerrillaMail API
    "moemail": {"api_url": None},    # Pulled from settings
}


def _generate_pkce():
    code_verifier = (
        base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b"=").decode("ascii")
    )
    code_challenge = (
        base64.urlsafe_b64encode(hashlib.sha256(code_verifier.encode("ascii")).digest())
        .rstrip(b"=")
        .decode("ascii")
    )
    return code_verifier, code_challenge


def _cookies_to_header(cookies):
    if hasattr(cookies, "get_dict"):
        cookie_dict = cookies.get_dict()
    else:
        try:
            cookie_dict = dict(cookies)
        except Exception:
            return str(cookies)
    return "; ".join([f"{k}={v}" for k, v in cookie_dict.items()])


def _oauth_device_flow(session, email, cookie_header=""):
    """Run the OAuth device flow to obtain an access_token."""
    client_id = "f0304373b74a44d2b584a3fb70ca9e56"
    scope = "openid profile email model.completion"
    code_verifier, code_challenge = _generate_pkce()

    device_resp = session.post(
        "https://chat.qwen.ai/api/v1/oauth2/device/code",
        data={
            "client_id": client_id,
            "scope": scope,
            "code_challenge": code_challenge,
            "code_challenge_method": "S256",
        },
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    if device_resp.status_code != 200:
        raise RuntimeError(
            f"Device code request failed [{device_resp.status_code}]: {device_resp.text[:200]}"
        )

    device_data = device_resp.json()
    device_code = device_data["device_code"]
    user_code = device_data.get("user_code", "")

    if user_code and cookie_header:
        auth_headers = {"Content-Type": "application/json", "Cookie": cookie_header}
        auth_resp = session.post(
            "https://chat.qwen.ai/api/v2/oauth2/authorize",
            json={"approved": True, "user_code": user_code},
            headers=auth_headers,
        )
        if auth_resp.status_code != 200:
            log.warning(f"[Register] OAuth authorization failed [{auth_resp.status_code}]")
        else:
            log.info(f"[Register] OAuth authorization succeeded: {email}")

    grant_type = "urn:ietf:params:oauth:grant-type:device_code"
    for attempt in range(60):
        time.sleep(5)
        token_resp = session.post(
            "https://chat.qwen.ai/api/v1/oauth2/token",
            data={
                "grant_type": grant_type,
                "client_id": client_id,
                "device_code": device_code,
                "code_verifier": code_verifier,
            },
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        )

        if token_resp.status_code == 200:
            oauth_data = token_resp.json()
            log.info(f"[Register] Token acquired: {email} (round {attempt + 1})")
            return oauth_data

        try:
            err_data = token_resp.json()
            err_code = err_data.get("error", "")
        except Exception:
            err_code = ""

        if err_code == "authorization_pending":
            continue
        if err_code == "slow_down":
            time.sleep(5)
            continue
        if err_code in ("expired_token", "access_denied"):
            log.warning(f"[Register] OAuth token rejected: {err_code}")
            break
        
        # Retry on 504 Gateway Timeout (Qwen infrastructure issue)
        if token_resp.status_code == 504:
            log.warning(f"[Register] Token polling 504 timeout, retrying... (attempt {attempt + 1}/60)")
            continue

        log.warning(f"[Register] Token polling error [{token_resp.status_code}]")
        break

    return None


def _register_single_account(provider: str = "default", moemail_domain: str = "", moemail_key: str = "",
                              tempmail_domain: str = "", tempmail_key: str = "",
                              mail_poll_times: int = 24) -> Optional[dict]:
    """
    Register a single Qwen account; returns the account dict or None.
    Synchronous function; designed to run inside a thread pool.
    """
    from curl_cffi import requests as curl_requests
    from lib.mail_service import MoeMailClient, TempMailClient, GuerrillaMailClient, LocalMailClient

    # 1. Obtain a temporary email address
    log.info("[Register] Obtaining a temporary email address...")
    verify_url_fetcher = None   # callable() -> str | None

    if provider == "local":
        # Local mailserver (snapsave.my.id)
        local = LocalMailClient()
        try:
            addr_info = local.create_address_sync()
        except Exception as e:
            log.error(f"[Register] LocalMail address creation failed: {e}")
            return None
        email_addr = addr_info["address"]
        log.info(f"[Register] LocalMail address acquired: {email_addr}")
        verify_url_fetcher = lambda: local.poll_for_activation_link(max_polls=mail_poll_times)

    elif provider == "moemail" and moemail_domain and moemail_key:
        # Self-hosted MoeMail
        moe = MoeMailClient(moemail_domain, moemail_key)
        try:
            addr_info = moe.create_address_sync()
        except Exception as e:
            log.error(f"[Register] MoeMail address creation failed: {e}")
            return None
        email_addr = addr_info["address"]
        email_id = addr_info["id"]
        log.info(f"[Register] MoeMail address acquired: {email_addr}")
        verify_url_fetcher = lambda: moe.poll_for_activation_link(email_id, max_polls=mail_poll_times)

    elif provider == "tempmail" and tempmail_domain and tempmail_key:
        # Self-hosted TempMail
        tmp = TempMailClient(tempmail_domain, tempmail_key)
        try:
            addr_info = tmp.create_address_sync()
        except Exception as e:
            log.error(f"[Register] TempMail address creation failed: {e}")
            return None
        email_addr = addr_info["address"]
        jwt = addr_info["jwt"]
        log.info(f"[Register] TempMail address acquired: {email_addr}")
        verify_url_fetcher = lambda: tmp.poll_for_activation_link(jwt, max_polls=mail_poll_times)

    elif provider == "guerrilla":
        # Official GuerrillaMail API
        gm = GuerrillaMailClient()
        try:
            addr_info = gm.create_address_sync()
        except Exception as e:
            log.error(f"[Register] GuerrillaMail address acquisition failed: {e}")
            return None
        email_addr = addr_info["address"]
        log.info(f"[Register] GuerrillaMail address acquired: {email_addr}")
        verify_url_fetcher = lambda: gm.poll_for_activation_link(max_polls=mail_poll_times)

    else:
        # Default channel: GPTMail (mail.chatgpt.org.uk) — automatically retrieves a public API key
        from lib.mail_service import GPTMailClient
        gptmail = GPTMailClient()
        try:
            addr_info = gptmail.create_address_sync()
        except Exception as e:
            log.error(f"[Register] GPTMail address acquisition failed: {e}")
            return None
        email_addr = addr_info["address"]
        log.info(f"[Register] GPTMail address acquired: {email_addr}")
        verify_url_fetcher = lambda: gptmail.poll_for_activation_link(email_addr, max_polls=mail_poll_times)

    # 2. Submit the registration form via headless browser
    from lib.browser_register import browser_signup_sync

    log.info(f"[Register] [{email_addr}] Calling browser_signup_sync...")
    signup_result = browser_signup_sync(email_addr, DEFAULT_PASSWORD_HASH)
    log.info(f"[Register] [{email_addr}] browser_signup_sync returned: {signup_result}")
    if not signup_result or not signup_result.get("success"):
        log.error(f"[Register] [{email_addr}] Form submission failed")
        return None

    browser_cookies = signup_result.get("cookies", {})
    log.info(f"[Register] [{email_addr}] Form submitted, waiting 10s for email to be sent...")
    time.sleep(10)  # Give Qwen time to send the activation email
    log.info(f"[Register] [{email_addr}] Now polling for activation email (up to {mail_poll_times} attempts, every 5s)")

    # 3. Wait for the verification email and click the activation link
    token_data = None
    cookie_header = ""

    try:
        from curl_cffi import requests as curl_requests
        with curl_requests.Session(impersonate="chrome119") as session:
            # Inject cookies captured by the browser
            for k, v in browser_cookies.items():
                session.cookies.set(k, v, domain=".chat.qwen.ai")

            verify_url = verify_url_fetcher()
            if not verify_url:
                log.error(f"[Register] {email_addr} verification email timed out")
                return None

            log.info(f"[Register] Activation link captured, activating {email_addr}...")
            session.get(url=verify_url)
            cookie_header = _cookies_to_header(session.cookies)

            # Check if we already have token from browser cookies
            browser_token = browser_cookies.get("token", "")
            
            # 4. OAuth device flow to obtain the token (only if not already available)
            if browser_token:
                log.info(f"[Register] Token already available from browser cookies, skipping OAuth")
                token_data = None
                jwt_token_from_cookie = browser_token
            else:
                log.info(f"[Register] Requesting token via OAuth: {email_addr}...")
                try:
                    token_data = _oauth_device_flow(session, email_addr, cookie_header=cookie_header)
                except Exception as e:
                    log.warning(f"[Register] Token acquisition failed: {email_addr}")
                    token_data = None

                jwt_token_from_cookie = session.cookies.get("token")

    except Exception as e:
        log.error(f"[Register] Registration process error {email_addr}: {e}")
        return None

    # Check token from browser cookies first (available immediately after signup)
    browser_token = browser_cookies.get("token", "")
    
    if not token_data and not jwt_token_from_cookie and not browser_token:
        log.error(f"[Register] {email_addr} no token obtained")
        return None

    # Extract the JWT token (priority: browser cookie > session cookie > OAuth)
    jwt_token = browser_token or jwt_token_from_cookie or (token_data.get("access_token", "") if token_data else "")

    if not jwt_token:
        log.error(f"[Register] {email_addr} token is empty")
        return None

    account_data = {
        "email": email_addr,
        "password": "AlIlzHZkJ4zG6J",
        "token": jwt_token,
        "cookies": cookie_header,
        "username": email_addr.split("@")[0],
        "activation_pending": False,
        "status_code": "valid",
        "last_error": "",
        "valid": True,
    }

    log.info(f"[Register] ✅ Registration succeeded: {email_addr}")
    return account_data


async def perform_batch_registration(
    account_pool,
    count: int = 1,
    threads: int = 4,
    provider: str = "default",
    moemail_domain: str = "",
    moemail_key: str = "",
    tempmail_domain: str = "",
    tempmail_key: str = "",
    stop_flag: "threading.Event | None" = None,
    max_retries: int = 24,  # Activation-email poll attempts (every 5s)
):
    """
    Bulk registration entry point.

    - count:       total number of account slots to attempt (success + failure = count)
    - max_retries: maximum activation-email polls per slot (every 5s); failure on timeout
    - threads:     number of slots running concurrently
    - stop_flag:   user-initiated stop signal
    """
    mail_poll_times = max(1, max_retries) if max_retries > 0 else 24
    max_workers = max(1, min(threads, 32))
    loop = asyncio.get_event_loop()
    success_count = 0
    fail_count = 0
    sem = asyncio.Semaphore(max_workers)

    log.info(
        f"[Register] Bulk registration start: total slots={count}, concurrency={threads}, mail polls={mail_poll_times} (i.e. {mail_poll_times * 5}s timeout), channel={provider}"
    )

    async def _run_slot(slot_num: int):
        """Run a single account slot once; failure on timeout."""
        nonlocal success_count, fail_count
        async with sem:
            if stop_flag and stop_flag.is_set():
                log.info(f"[Register] Slot {slot_num}/{count} received stop signal, exiting")
                return

            result = await loop.run_in_executor(
                None,
                _register_single_account,
                provider,
                moemail_domain,
                moemail_key,
                tempmail_domain,
                tempmail_key,
                mail_poll_times,
            )

            if result and result.get("token"):
                await account_pool.add_account(
                    email=result["email"],
                    password=result.get("password", ""),
                    token=result["token"],
                )
                success_count += 1
                log.info(
                    f"[Register] Slot {slot_num}/{count} ✅ registered, added to pool (total success {success_count})"
                )
            else:
                fail_count += 1
                log.warning(f"[Register] Slot {slot_num}/{count} ❌ registration failed")

    # Create all slot tasks at once; concurrency is bounded by the semaphore
    all_tasks = [asyncio.create_task(_run_slot(i + 1)) for i in range(count)]
    await asyncio.gather(*all_tasks, return_exceptions=True)

    stopped = stop_flag and stop_flag.is_set()
    status = "manually stopped" if stopped else "completed"
    log.info(
        f"[Register] Bulk registration {status}: "
        f"success={success_count}, failed={fail_count}, total slots={count}"
    )
    return {"success": success_count, "failed": fail_count, "stopped": bool(stopped)}
