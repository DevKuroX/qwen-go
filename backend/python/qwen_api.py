#!/usr/bin/env python3
"""
JSON-RPC Qwen API client for Golang subprocess integration.
Usage: python qwen_api.py --json-rpc
"""

import sys
import json
import asyncio
import logging

logging.basicConfig(level=logging.INFO, format='%(message)s')

from lib.qwen_client import QwenClient
from lib.account_pool import AccountPool
from lib.database import AsyncJsonDB
from lib.config import settings


async def main():
    if "--json-rpc" not in sys.argv:
        print("Usage: python qwen_api.py --json-rpc", file=sys.stderr)
        sys.exit(1)
    
    try:
        request = json.loads(sys.stdin.readline())
    except Exception as e:
        response = {"success": False, "error": f"Failed to parse request: {e}"}
        print(json.dumps(response))
        sys.stdout.flush()
        sys.exit(1)
    
    action = request.get("action", "chat")
    token = request.get("token", "")
    model = request.get("model", "qwen3.6-plus")
    messages = request.get("messages", [])
    prompt = request.get("prompt", "")
    
    try:
        client = QwenClient(None, None)
        
        if action == "chat":
            chat_id = await client.create_chat(token, model)
            
            payload = {
                "chat_type": "t2t",
                "messages": messages,
                "model": model,
            }
            
            content_parts = []
            async for chunk in client.engine.fetch_chat(token, chat_id, payload, buffered=False):
                if chunk.get("status") not in (200, "streamed"):
                    raise Exception(f"HTTP {chunk.get('status')}: {chunk.get('body', '')[:200]}")
                
                if "chunk" in chunk:
                    content_parts.append(chunk["chunk"])
            
            await client.delete_chat(token, chat_id)
            
            full_content = "".join(content_parts)
            
            response = {
                "success": True,
                "content": full_content,
            }
        
        elif action == "image":
            chat_id, answer_text = await client.image_generate_with_retry(model, prompt)
            await client.delete_chat(token, chat_id)
            
            import re
            urls = re.findall(r'https://[^\s"\'<>\)]+', answer_text)
            
            response = {
                "success": True,
                "image_url": urls[0] if urls else None,
                "raw_response": answer_text,
            }
        
        else:
            response = {"success": False, "error": f"Unknown action: {action}"}
    
    except Exception as e:
        response = {"success": False, "error": str(e)}
    
    print(json.dumps(response, default=str))
    sys.stdout.flush()


if __name__ == "__main__":
    asyncio.run(main())
