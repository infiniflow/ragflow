import aiohttp
import json
import asyncio
import textwrap
import base64
import time

host = "http://localhost:9385"
host_ws = "ws://localhost:9385"
_CONCURRENT_TASKS = 100

def code(        
) -> str:
    return"""
def main():
    print("Hello World")
"""

def create_model(
    code: str,
) -> dict:
    return {
        "code_b64" : textwrap.dedent(base64.b64encode(code.encode("utf-8")).decode("utf-8")),
        "language" : "python",
        "arguments": {},
    }

async def websocket_communication(
    message: dict,
    num: int,      
) -> None:
    start_time = time.time()
    async with aiohttp.ClientSession() as session, session.ws_connect(f"{host_ws}/run_ws") as ws:
        await ws.send_str(json.dumps(message))
        async for msg in ws:
            print(f"Server {num + 1}: {msg.data}")
            if "status" in msg.data:
                print(f"stdout {num + 1}: {json.loads(msg.data)['stdout']}")
                break
        print(f"Coroutine {num + 1} runtime : {time.time()-start_time}")

async def http_communication(
    message: dict,
    num: int,      
) -> None:
    start_time = time.time()
    async with aiohttp.ClientSession() as session, session.post(
        f"{host}/run",
        json=message,
    ) as response:
        print(f"Coroutine  {num+1} Response : {await response.text()}")
        print(f"Coroutine  {num+1} runtime : {time.time()-start_time}")

async def main():
    message = create_model(code=code())
    websocket_list = []
    http_list = []
    for i in range(_CONCURRENT_TASKS):
        websocket_list.append(websocket_communication(message=message,num=i))
    await asyncio.gather(*websocket_list)
    for i in range(_CONCURRENT_TASKS):
        http_list.append(http_communication(message=message,num=i))
    await asyncio.gather(*http_list)

asyncio.run(main())