#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from jina import Deployment
from docarray import BaseDoc
from jina import Executor, requests
from transformers import AutoModelForCausalLM, AutoTokenizer, GenerationConfig
import argparse
import torch


class Prompt(BaseDoc):
    message: list[dict]
    gen_conf: dict


class Generation(BaseDoc):
    text: str


tokenizer = None
model_name = ""


class TokenStreamingExecutor(Executor):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.model = AutoModelForCausalLM.from_pretrained(
            model_name, device_map="auto", torch_dtype="auto"
        )

    @requests(on="/chat")
    async def generate(self, doc: Prompt, **kwargs) -> Generation:
        text = tokenizer.apply_chat_template(
            doc.message,
            tokenize=False,
        )
        inputs = tokenizer([text], return_tensors="pt")
        generation_config = GenerationConfig(
            **doc.gen_conf,
            eos_token_id=tokenizer.eos_token_id,
            pad_token_id=tokenizer.eos_token_id
        )
        generated_ids = self.model.generate(
            inputs.input_ids, generation_config=generation_config
        )
        generated_ids = [
            output_ids[len(input_ids) :]
            for input_ids, output_ids in zip(inputs.input_ids, generated_ids)
        ]

        response = tokenizer.batch_decode(generated_ids, skip_special_tokens=True)[0]
        yield Generation(text=response)

    @requests(on="/stream")
    async def task(self, doc: Prompt, **kwargs) -> Generation:
        text = tokenizer.apply_chat_template(
            doc.message,
            tokenize=False,
        )
        input = tokenizer([text], return_tensors="pt")
        input_len = input["input_ids"].shape[1]
        max_new_tokens = 512
        if "max_new_tokens" in doc.gen_conf:
            max_new_tokens = doc.gen_conf.pop("max_new_tokens")
        generation_config = GenerationConfig(
            **doc.gen_conf,
            eos_token_id=tokenizer.eos_token_id,
            pad_token_id=tokenizer.eos_token_id
        )
        for _ in range(max_new_tokens):
            output = self.model.generate(
                **input, max_new_tokens=1, generation_config=generation_config
            )
            if output[0][-1] == tokenizer.eos_token_id:
                break
            yield Generation(
                text=tokenizer.decode(output[0][input_len:], skip_special_tokens=True)
            )
            input = {
                "input_ids": output,
                "attention_mask": torch.ones(1, len(output[0])),
            }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--model_name", type=str, help="Model name or path")
    parser.add_argument("--port", default=12345, type=int, help="Jina serving port")
    args = parser.parse_args()
    model_name = args.model_name
    tokenizer = AutoTokenizer.from_pretrained(args.model_name)
    with Deployment(
        uses=TokenStreamingExecutor, port=args.port, protocol="grpc"
    ) as dep:
        dep.block()
