#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import logging
import discord
import aiohttp
import base64
import io
import time
import asyncio

logger = logging.getLogger(__name__)

URL = '{YOUR_IP_ADDRESS:PORT}/v1/api/completion_aibotk'  # Default: https://cloud.ragflow.io/v1/api/completion_aibotk

JSON_DATA = {
    "conversation_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxx",  # Get conversation id from /api/new_conversation
    "Authorization": "ragflow-xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",  # RAGFlow Assistant Chat Bot API Key
    "word": ""  # User question, don't need to initialize
}

DISCORD_BOT_KEY = "xxxxxxxxxxxxxxxxxxxxxxxxxx"  # Get DISCORD_BOT_KEY from Discord Application

intents = discord.Intents.default()
intents.message_content = True
client = discord.Client(intents=intents)


@client.event
async def on_ready():
    logger.info(f'We have logged in as {client.user}')


@client.event
async def on_message(message):
    if message.author == client.user:
        return

    if client.user.mentioned_in(message):

        if len(message.content.split('> ')) == 1:
            await message.channel.send("Hi~ How can I help you? ")
        else:
            # Now that the request is awaited, two overlapping `on_message`
            # handlers would race on a shared dict: one could overwrite `word`
            # before the other's request is sent. Build a per-request payload.
            payload = {**JSON_DATA, 'word': message.content.split('> ')[1]}
            started_at = time.monotonic()
            async with aiohttp.ClientSession() as session:
                async with session.post(URL, json=payload) as response:
                    elapsed = time.monotonic() - started_at
                    logger.info(
                        'Completion request finished with status %s in %.3fs',
                        response.status, elapsed
                    )
                    response_data = (await response.json()).get('data', [])
            image_bool = False

            for i in response_data:
                if i['type'] == 1:
                    res = i['content']
                if i['type'] == 3:
                    image_bool = True
                    image_data = base64.b64decode(i['url'])
                    image = discord.File(io.BytesIO(image_data), filename='image.png')
                    logger.info('Built image attachment of %d bytes', len(image_data))

            await message.channel.send(f"{message.author.mention}{res}")

            if image_bool:
                try:
                    await message.channel.send(file=image)
                except Exception:
                    logger.exception('Failed to send the image attachment')
                else:
                    logger.info('Image attachment sent')


loop = asyncio.get_event_loop()

try:
    loop.run_until_complete(client.start(DISCORD_BOT_KEY))
except KeyboardInterrupt:
    loop.run_until_complete(client.close())
finally:
    loop.close()
