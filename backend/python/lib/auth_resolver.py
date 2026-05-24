"""
auth_resolver.py — Account auto-recovery
When account authentication fails or is pending activation, attempts to automatically re-login to obtain a new token.
"""

import asyncio
import logging

log = logging.getLogger("qwenpi.auth_resolver")

BASE_URL = "https://chat.qwen.ai"


class AuthResolver:
    """Attempts to automatically recover the token for a failed account."""

    def __init__(self, account_pool):
        self.account_pool = account_pool
        self._healing: set[str] = set()  # Emails currently being healed

    async def auto_heal_account(self, acc):
        """Attempts to re-login and refresh the token in the background."""
        if acc.email in self._healing:
            log.info(f"[AuthResolver] {acc.email} is already in the recovery queue, skipping")
            return
        self._healing.add(acc.email)
        try:
            log.info(f"[AuthResolver] Starting recovery for account: {acc.email}")

            if not acc.password:
                log.warning(f"[AuthResolver] {acc.email} has no password, cannot auto-recover")
                return

            new_token = await self._try_login(acc.email, acc.password)
            if new_token:
                acc.token = new_token
                self.account_pool.mark_valid(acc)
                await self.account_pool.save()
                log.info(f"[AuthResolver] {acc.email} recovery succeeded")
            else:
                log.warning(f"[AuthResolver] {acc.email} recovery failed")
        except Exception as e:
            log.error(f"[AuthResolver] {acc.email} recovery exception: {e}")
        finally:
            self._healing.discard(acc.email)

    async def _try_login(self, email: str, password: str) -> str | None:
        """Attempts to obtain a new token via the Qwen login API."""
        try:
            import httpx
            async with httpx.AsyncClient(timeout=30) as client:
                resp = await client.post(
                    f"{BASE_URL}/api/v1/auths/signin",
                    json={"email": email, "password": password},
                    headers={
                        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
                        "Content-Type": "application/json",
                        "Referer": f"{BASE_URL}/",
                        "Origin": BASE_URL,
                    }
                )
                if resp.status_code == 200:
                    data = resp.json()
                    return data.get("token") or data.get("access_token")
                log.warning(f"[AuthResolver] Login failed HTTP {resp.status_code}: {resp.text[:100]}")
                return None
        except Exception as e:
            log.error(f"[AuthResolver] Login request exception: {e}")
            return None
