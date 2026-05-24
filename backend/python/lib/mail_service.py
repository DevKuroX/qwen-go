"""
mail_service.py — Temporary email service clients
Provides clients for various temporary email providers used during account registration.
"""

import logging
import time
import re
from typing import Optional, Dict
from curl_cffi import requests as curl_requests

log = logging.getLogger("qwenpi.mail_service")


class GuerrillaMailClient:
    """Client for GuerrillaMail API (https://www.guerrillamail.com/GuerrillaMailAPI.html)"""
    
    BASE_URL = "https://api.guerrillamail.com/ajax.php"
    
    def __init__(self):
        self.session = curl_requests.Session(impersonate="chrome119")
        self.email_addr = None
        self.sid_token = None
    
    def create_address_sync(self) -> Dict[str, str]:
        """Create a new temporary email address"""
        try:
            resp = self.session.get(
                self.BASE_URL,
                params={"f": "get_email_address"},
                timeout=15
            )
            resp.raise_for_status()
            data = resp.json()
            
            self.email_addr = data.get("email_addr")
            self.sid_token = data.get("sid_token")
            
            if not self.email_addr:
                raise RuntimeError("GuerrillaMail did not return email address")
            
            log.info(f"[GuerrillaMail] Created address: {self.email_addr}")
            return {"address": self.email_addr, "sid": self.sid_token}
        
        except Exception as e:
            log.error(f"[GuerrillaMail] Failed to create address: {e}")
            raise
    
    def poll_for_activation_link(self, max_polls: int = 24) -> Optional[str]:
        """Poll for activation email and extract verification link"""
        if not self.sid_token:
            log.error("[GuerrillaMail] No sid_token available")
            return None
        
        log.info(f"[GuerrillaMail] Polling for activation email (max {max_polls} attempts, 5s interval)")
        
        for attempt in range(max_polls):
            try:
                resp = self.session.get(
                    self.BASE_URL,
                    params={
                        "f": "check_email",
                        "sid_token": self.sid_token,
                        "seq": 0
                    },
                    timeout=15
                )
                resp.raise_for_status()
                data = resp.json()
                
                email_list = data.get("list", [])
                
                for email in email_list:
                    mail_subject = email.get("mail_subject", "")
                    mail_id = email.get("mail_id")
                    
                    # Check if this is the Qwen activation email
                    if "qwen" in mail_subject.lower() or "verify" in mail_subject.lower() or "activation" in mail_subject.lower():
                        log.info(f"[GuerrillaMail] Found activation email: {mail_subject}")
                        
                        # Fetch full email body
                        body_resp = self.session.get(
                            self.BASE_URL,
                            params={
                                "f": "fetch_email",
                                "sid_token": self.sid_token,
                                "email_id": mail_id
                            },
                            timeout=15
                        )
                        body_resp.raise_for_status()
                        body_data = body_resp.json()
                        
                        mail_body = body_data.get("mail_body", "")
                        
                        # Extract verification link
                        verify_link = self._extract_verification_link(mail_body)
                        if verify_link:
                            log.info(f"[GuerrillaMail] Extracted verification link")
                            return verify_link
                
                if attempt < max_polls - 1:
                    time.sleep(5)
            
            except Exception as e:
                log.warning(f"[GuerrillaMail] Poll attempt {attempt + 1} failed: {e}")
                if attempt < max_polls - 1:
                    time.sleep(5)
        
        log.error(f"[GuerrillaMail] Activation email not received after {max_polls} attempts")
        return None
    
    def _extract_verification_link(self, mail_body: str) -> Optional[str]:
        """Extract verification link from email HTML (handles base64 encoded content)"""
        import base64
        
        # First try to find base64 encoded HTML content
        b64_match = re.search(r'Content-Transfer-Encoding: base64\r?\n\r?\n([A-Za-z0-9+/=\r\n]+)', mail_body)
        if b64_match:
            try:
                b64_content = b64_match.group(1).replace('\r\n', '').replace('\n', '')
                decoded_html = base64.b64decode(b64_content).decode('utf-8', errors='ignore')
                mail_body = decoded_html
            except Exception as e:
                log.warning(f"[LocalMail] Failed to decode base64 content: {e}")
        
        # Look for activation links (both verify and activate)
        patterns = [
            r'https://chat\.qwen\.ai/api/v1/auths/activate\?[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/api/v1/auths/verify\?[^\s"\'<>]+',
            r'href=["\']([^"\']+(?:activate|verify)[^"\']+)["\']',
        ]
        
        for pattern in patterns:
            match = re.search(pattern, mail_body, re.IGNORECASE)
            if match:
                link = match.group(1) if match.lastindex and match.lastindex > 0 else match.group(0)
                link = link.replace("&amp;", "&")
                log.info(f"[LocalMail] Extracted link: {link[:80]}...")
                return link
        
        log.warning(f"[LocalMail] No activation link found in email body (length: {len(mail_body)})")
        return None


class GPTMailClient:
    """Client for GPTMail (mail.chatgpt.org.uk)"""
    
    BASE_URL = "https://mail.chatgpt.org.uk"
    
    def __init__(self):
        self.session = curl_requests.Session(impersonate="chrome119")
        self.email_addr = None
        self.api_key = None
    
    def create_address_sync(self) -> Dict[str, str]:
        """Create a new temporary email address"""
        try:
            # First, get an API key
            key_resp = self.session.get(f"{self.BASE_URL}/api/key", timeout=15)
            key_resp.raise_for_status()
            self.api_key = key_resp.json().get("key")
            
            if not self.api_key:
                raise RuntimeError("GPTMail did not return API key")
            
            # Create email address
            resp = self.session.post(
                f"{self.BASE_URL}/api/email",
                headers={"Authorization": f"Bearer {self.api_key}"},
                timeout=15
            )
            resp.raise_for_status()
            data = resp.json()
            
            self.email_addr = data.get("email")
            
            if not self.email_addr:
                raise RuntimeError("GPTMail did not return email address")
            
            log.info(f"[GPTMail] Created address: {self.email_addr}")
            return {"address": self.email_addr, "key": self.api_key}
        
        except Exception as e:
            log.error(f"[GPTMail] Failed to create address: {e}")
            raise
    
    def poll_for_activation_link(self, email_addr: str, max_polls: int = 24) -> Optional[str]:
        """Poll for activation email and extract verification link"""
        if not self.api_key:
            log.error("[GPTMail] No API key available")
            return None
        
        log.info(f"[GPTMail] Polling for activation email (max {max_polls} attempts, 5s interval)")
        
        for attempt in range(max_polls):
            try:
                resp = self.session.get(
                    f"{self.BASE_URL}/api/messages/{email_addr}",
                    headers={"Authorization": f"Bearer {self.api_key}"},
                    timeout=15
                )
                resp.raise_for_status()
                messages = resp.json()
                
                for msg in messages:
                    subject = msg.get("subject", "")
                    
                    # Check if this is the Qwen activation email
                    if "qwen" in subject.lower() or "verify" in subject.lower() or "activation" in subject.lower():
                        log.info(f"[GPTMail] Found activation email: {subject}")
                        
                        msg_id = msg.get("id")
                        
                        # Fetch full message
                        body_resp = self.session.get(
                            f"{self.BASE_URL}/api/message/{msg_id}",
                            headers={"Authorization": f"Bearer {self.api_key}"},
                            timeout=15
                        )
                        body_resp.raise_for_status()
                        body_data = body_resp.json()
                        
                        mail_body = body_data.get("html", "") or body_data.get("text", "")
                        
                        # Extract verification link
                        verify_link = self._extract_verification_link(mail_body)
                        if verify_link:
                            log.info(f"[GPTMail] Extracted verification link")
                            return verify_link
                
                if attempt < max_polls - 1:
                    time.sleep(5)
            
            except Exception as e:
                log.warning(f"[GPTMail] Poll attempt {attempt + 1} failed: {e}")
                if attempt < max_polls - 1:
                    time.sleep(5)
        
        log.error(f"[GPTMail] Activation email not received after {max_polls} attempts")
        return None
    
    def _extract_verification_link(self, html_body: str) -> Optional[str]:
        """Extract verification link from email HTML"""
        patterns = [
            r'https://chat\.qwen\.ai/[^\s"\'<>]+verify[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/api/v1/auths/verify[^\s"\'<>]+',
            r'href=["\']([^"\']+verify[^"\']+)["\']',
        ]
        
        for pattern in patterns:
            match = re.search(pattern, html_body, re.IGNORECASE)
            if match:
                link = match.group(1) if match.lastindex else match.group(0)
                link = link.replace("&amp;", "&")
                return link
        
        return None


class MoeMailClient:
    """Client for self-hosted MoeMail"""
    
    def __init__(self, domain: str, api_key: str):
        self.domain = domain.rstrip("/")
        self.api_key = api_key
        self.session = curl_requests.Session(impersonate="chrome119")
    
    def create_address_sync(self) -> Dict[str, str]:
        """Create a new temporary email address"""
        try:
            resp = self.session.post(
                f"{self.domain}/api/address",
                headers={"Authorization": f"Bearer {self.api_key}"},
                timeout=15
            )
            resp.raise_for_status()
            data = resp.json()
            
            email_addr = data.get("address")
            email_id = data.get("id")
            
            if not email_addr:
                raise RuntimeError("MoeMail did not return email address")
            
            log.info(f"[MoeMail] Created address: {email_addr}")
            return {"address": email_addr, "id": email_id}
        
        except Exception as e:
            log.error(f"[MoeMail] Failed to create address: {e}")
            raise
    
    def poll_for_activation_link(self, email_id: str, max_polls: int = 24) -> Optional[str]:
        """Poll for activation email and extract verification link"""
        log.info(f"[MoeMail] Polling for activation email (max {max_polls} attempts, 5s interval)")
        
        for attempt in range(max_polls):
            try:
                resp = self.session.get(
                    f"{self.domain}/api/messages/{email_id}",
                    headers={"Authorization": f"Bearer {self.api_key}"},
                    timeout=15
                )
                resp.raise_for_status()
                messages = resp.json()
                
                for msg in messages:
                    subject = msg.get("subject", "")
                    
                    if "qwen" in subject.lower() or "verify" in subject.lower() or "activation" in subject.lower():
                        log.info(f"[MoeMail] Found activation email: {subject}")
                        
                        mail_body = msg.get("html", "") or msg.get("text", "")
                        
                        verify_link = self._extract_verification_link(mail_body)
                        if verify_link:
                            log.info(f"[MoeMail] Extracted verification link")
                            return verify_link
                
                if attempt < max_polls - 1:
                    time.sleep(5)
            
            except Exception as e:
                log.warning(f"[MoeMail] Poll attempt {attempt + 1} failed: {e}")
                if attempt < max_polls - 1:
                    time.sleep(5)
        
        log.error(f"[MoeMail] Activation email not received after {max_polls} attempts")
        return None
    
    def _extract_verification_link(self, html_body: str) -> Optional[str]:
        """Extract verification link from email HTML"""
        patterns = [
            r'https://chat\.qwen\.ai/[^\s"\'<>]+verify[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/api/v1/auths/verify[^\s"\'<>]+',
            r'href=["\']([^"\']+verify[^"\']+)["\']',
        ]
        
        for pattern in patterns:
            match = re.search(pattern, html_body, re.IGNORECASE)
            if match:
                link = match.group(1) if match.lastindex else match.group(0)
                link = link.replace("&amp;", "&")
                return link
        
        return None


class LocalMailClient:
    """Client for local mailserver (snapsave.my.id)"""
    
    BASE_URL = "http://localhost:8080"
    DOMAIN = "snapsave.my.id"
    
    def __init__(self):
        self.session = curl_requests.Session(impersonate="chrome119")
        self.email_addr = None
    
    def create_address_sync(self) -> Dict[str, str]:
        """Create a new temporary email address"""
        import random
        import string
        
        # Generate random username
        username = ''.join(random.choices(string.ascii_lowercase + string.digits, k=8))
        self.email_addr = f"{username}@{self.DOMAIN}"
        
        log.info(f"[LocalMail] Created address: {self.email_addr}")
        return {"address": self.email_addr}
    
    def poll_for_activation_link(self, max_polls: int = 24) -> Optional[str]:
        """Poll for activation email and extract verification link"""
        if not self.email_addr:
            log.error("[LocalMail] No email address available")
            return None
        
        log.info(f"[LocalMail] Polling for activation email (max {max_polls} attempts, 5s interval)")
        
        for attempt in range(max_polls):
            try:
                resp = self.session.get(
                    f"{self.BASE_URL}/api/email/{self.email_addr}",
                    timeout=10
                )
                
                if resp.status_code == 200:
                    data = resp.json()
                    subject = data.get("subject", "")
                    
                    # Check if this is the Qwen activation email
                    if "qwen" in subject.lower() or "verify" in subject.lower() or "activation" in subject.lower():
                        log.info(f"[LocalMail] Found activation email: {subject}")
                        
                        mail_body = data.get("html", "") or data.get("text", "")
                        
                        # Extract verification link
                        verify_link = self._extract_verification_link(mail_body)
                        if verify_link:
                            log.info(f"[LocalMail] Extracted verification link")
                            return verify_link
                
                if attempt < max_polls - 1:
                    time.sleep(5)
            
            except Exception as e:
                log.warning(f"[LocalMail] Poll attempt {attempt + 1} failed: {e}")
                if attempt < max_polls - 1:
                    time.sleep(5)
        
        log.error(f"[LocalMail] Activation email not received after {max_polls} attempts")
        return None
    
    def _extract_verification_link(self, html_body: str) -> Optional[str]:
        """Extract verification link from email HTML (handles base64 encoded content)"""
        import base64
        
        # First try to decode base64 if present
        decoded_body = html_body
        b64_match = re.search(r'Content-Transfer-Encoding: base64\r?\n\r?\n([A-Za-z0-9+/=\r\n]+)', html_body)
        if b64_match:
            try:
                b64_content = b64_match.group(1).replace('\r\n', '').replace('\n', '')
                decoded_body = base64.b64decode(b64_content).decode('utf-8', errors='ignore')
                log.info("[LocalMail] Decoded base64 email content")
            except Exception as e:
                log.warning(f"[LocalMail] Failed to decode base64: {e}")
        
        # Look for activation/verify links
        patterns = [
            r'https://chat\.qwen\.ai/api/v1/auths/activate[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/api/v1/auths/verify[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/[^\s"\'<>]+activate[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/[^\s"\'<>]+verify[^\s"\'<>]+',
            r'href=["\']([^"\']+(?:activate|verify)[^"\']+)["\']',
        ]
        
        for pattern in patterns:
            match = re.search(pattern, decoded_body, re.IGNORECASE)
            if match:
                link = match.group(1) if match.lastindex else match.group(0)
                link = link.replace("&amp;", "&")
                log.info(f"[LocalMail] Found activation link: {link[:80]}...")
                return link
        
        log.warning("[LocalMail] No activation link found in email")
        return None


class TempMailClient:
    """Client for self-hosted TempMail (Cloudflare Workers)"""
    
    def __init__(self, domain: str, admin_password: str):
        self.domain = domain.rstrip("/")
        self.admin_password = admin_password
        self.session = curl_requests.Session(impersonate="chrome119")
    
    def create_address_sync(self) -> Dict[str, str]:
        """Create a new temporary email address"""
        try:
            resp = self.session.post(
                f"{self.domain}/api/new",
                headers={"X-Admin-Password": self.admin_password},
                timeout=15
            )
            resp.raise_for_status()
            data = resp.json()
            
            email_addr = data.get("address")
            jwt = data.get("jwt")
            
            if not email_addr:
                raise RuntimeError("TempMail did not return email address")
            
            log.info(f"[TempMail] Created address: {email_addr}")
            return {"address": email_addr, "jwt": jwt}
        
        except Exception as e:
            log.error(f"[TempMail] Failed to create address: {e}")
            raise
    
    def poll_for_activation_link(self, jwt: str, max_polls: int = 24) -> Optional[str]:
        """Poll for activation email and extract verification link"""
        log.info(f"[TempMail] Polling for activation email (max {max_polls} attempts, 5s interval)")
        
        for attempt in range(max_polls):
            try:
                resp = self.session.get(
                    f"{self.domain}/api/messages",
                    headers={"Authorization": f"Bearer {jwt}"},
                    timeout=15
                )
                resp.raise_for_status()
                messages = resp.json()
                
                for msg in messages:
                    subject = msg.get("subject", "")
                    
                    if "qwen" in subject.lower() or "verify" in subject.lower() or "activation" in subject.lower():
                        log.info(f"[TempMail] Found activation email: {subject}")
                        
                        mail_body = msg.get("html", "") or msg.get("text", "")
                        
                        verify_link = self._extract_verification_link(mail_body)
                        if verify_link:
                            log.info(f"[TempMail] Extracted verification link")
                            return verify_link
                
                if attempt < max_polls - 1:
                    time.sleep(5)
            
            except Exception as e:
                log.warning(f"[TempMail] Poll attempt {attempt + 1} failed: {e}")
                if attempt < max_polls - 1:
                    time.sleep(5)
        
        log.error(f"[TempMail] Activation email not received after {max_polls} attempts")
        return None
    
    def _extract_verification_link(self, html_body: str) -> Optional[str]:
        """Extract verification link from email HTML"""
        patterns = [
            r'https://chat\.qwen\.ai/[^\s"\'<>]+verify[^\s"\'<>]+',
            r'https://chat\.qwen\.ai/api/v1/auths/verify[^\s"\'<>]+',
            r'href=["\']([^"\']+verify[^"\']+)["\']',
        ]
        
        for pattern in patterns:
            match = re.search(pattern, html_body, re.IGNORECASE)
            if match:
                link = match.group(1) if match.lastindex else match.group(0)
                link = link.replace("&amp;", "&")
                return link
        
        return None
