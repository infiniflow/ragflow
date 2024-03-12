import argparse
import pickle
import random
import time
from multiprocessing.connection import Listener
from threading import Thread
import torch


class RPCHandler:
    def __init__(self):
        self._functions = { }

    def register_function(self, func):
        self._functions[func.__name__] = func

    def handle_connection(self, connection):
        try:
            while True:
                # Receive a message
                func_name, args, kwargs = pickle.loads(connection.recv())
                # Run the RPC and send a response
                try:
                    r = self._functions[func_name](*args,**kwargs)
                    connection.send(pickle.dumps(r))
                except Exception as e:
                    connection.send(pickle.dumps(e))
        except EOFError:
             pass


def rpc_server(hdlr, address, authkey):
    sock = Listener(address, authkey=authkey)
    while True:
        try:
            client = sock.accept()
            t = Thread(target=hdlr.handle_connection, args=(client,))
            t.daemon = True
            t.start()
        except Exception as e:
            print("【EXCEPTION】:", str(e))


models = []
tokenizer = None

def chat(messages, gen_conf):
    global tokenizer
    model = Model()
    roles = {"system":"System", "user": "User", "assistant": "Assistant"}
    line = ["{}: {}".format(roles[m["role"].lower()], m["content"]) for m in messages]
    line = "\n".join(line) + "\nAssistant: "
    tokens = tokenizer([line], return_tensors='pt')
    tokens = {k: tokens[k].to(model.device) if isinstance(tokens[k], torch.Tensor) else tokens[k] for k in
              tokens.keys()}
    res = [tokenizer.decode(t) for t in model.generate(**tokens, **gen_conf)][0]
    return res.split("Assistant: ")[-1]


def Model():
    global models
    random.seed(time.time())
    return random.choice(models)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--model_name", type=str, help="Model name")
    parser.add_argument("--port", default=7860, type=int, help="RPC serving port")
    args = parser.parse_args()

    handler = RPCHandler()
    handler.register_function(chat)

    from transformers import AutoModelForCausalLM, AutoTokenizer
    from transformers.generation.utils import GenerationConfig

    models = []
    for _ in range(2):
        m = AutoModelForCausalLM.from_pretrained(args.model_name,
                                                 device_map="auto",
                                                 torch_dtype='auto',
                                                 trust_remote_code=True)
        m.generation_config = GenerationConfig.from_pretrained(args.model_name)
        m.generation_config.pad_token_id = m.generation_config.eos_token_id
        models.append(m)
    tokenizer = AutoTokenizer.from_pretrained(args.model_name, use_fast=False,
                                              trust_remote_code=True)

    # Run the server
    rpc_server(handler, ('0.0.0.0', args.port), authkey=b'infiniflow-token4kevinhu')
