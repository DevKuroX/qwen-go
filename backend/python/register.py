#!/usr/bin/env python3
"""
JSON-RPC registration script for Golang subprocess integration.
Usage: python register.py --json-rpc
Input: JSON from stdin
Output: JSON to stdout
"""

import sys
import json
import asyncio
import logging

logging.basicConfig(level=logging.INFO, format='%(asctime)s %(name)s %(message)s')
log = logging.getLogger("register")

from lib.register import _register_single_account


def main():
    if "--json-rpc" not in sys.argv:
        print("Usage: python register.py --json-rpc", file=sys.stderr)
        sys.exit(1)
    
    try:
        request = json.loads(sys.stdin.readline())
    except Exception as e:
        response = {
            "success": False,
            "accounts": [],
            "error": f"Failed to parse request: {e}"
        }
        print(json.dumps(response))
        sys.stdout.flush()
        sys.exit(1)
    
    count = request.get("count", 1)
    threads = request.get("threads", 1)
    provider = request.get("provider", "guerrilla")
    moemail_domain = request.get("moemail_domain", "")
    moemail_key = request.get("moemail_key", "")
    tempmail_domain = request.get("tempmail_domain", "")
    tempmail_key = request.get("tempmail_key", "")
    
    accounts = []
    errors = []
    
    for i in range(count):
        log.info(f"Registering account {i+1}/{count}...")
        
        try:
            result = _register_single_account(
                provider=provider,
                moemail_domain=moemail_domain,
                moemail_key=moemail_key,
                tempmail_domain=tempmail_domain,
                tempmail_key=tempmail_key,
                mail_poll_times=10
            )
            
            if result:
                accounts.append(result)
                log.info(f"Account registered: {result['email']}")
            else:
                errors.append(f"Registration {i+1} returned None")
                log.warning(f"Registration {i+1} failed")
        except Exception as e:
            errors.append(str(e))
            log.error(f"Registration {i+1} error: {e}")
    
    response = {
        "success": len(accounts) > 0,
        "accounts": accounts,
        "error": "; ".join(errors) if errors else None
    }
    
    print(json.dumps(response, default=str))
    sys.stdout.flush()


if __name__ == "__main__":
    main()
