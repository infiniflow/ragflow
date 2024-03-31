import argparse
import pickle
import random
import time
from multiprocessing.connection import Listener
from threading import Thread
from transformers import AutoModelForCausalLM, AutoTokenizer


def torch_gc():
    try:
        import torch
        if torch.cuda.is_available():
            # with torch.cuda.device(DEVICE):
            torch.cuda.empty_cache()
            torch.cuda.ipc_collect()
        elif torch.backends.mps.is_available():
            try:
                from torch.mps import empty_cache
                empty_cache()
            except Exception as e:
                pass
    except Exception:
        pass


class RPCHandler:
    def __init__(self):
        self._functions = {}

    def register_function(self, func):
        self._functions[func.__name__] = func

    def handle_connection(self, connection):
        try:
            while True:
                # Receive a message
                func_name, args, kwargs = pickle.loads(connection.recv())
                # Run the RPC and send a response
                try:
                    r = self._functions[func_name](*args, **kwargs)
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
    try:
        torch_gc()
        conf = {
            "max_new_tokens": int(
                gen_conf.get(
                    "max_tokens", 256)), "temperature": float(
                gen_conf.get(
                    "temperature", 0.1))}
        print(messages, conf)
        text = tokenizer.apply_chat_template(
            messages,
            tokenize=False,
            add_generation_prompt=True
        )
        model_inputs = tokenizer([text], return_tensors="pt").to(model.device)

        generated_ids = model.generate(
            model_inputs.input_ids,
            **conf
        )
        generated_ids = [
            output_ids[len(input_ids):] for input_ids, output_ids in zip(model_inputs.input_ids, generated_ids)
        ]

        return tokenizer.batch_decode(
            generated_ids, skip_special_tokens=True)[0]
    except Exception as e:
        return str(e)


def Model():
    global models
    random.seed(time.time())
    return random.choice(models)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--model_name", type=str, help="Model name")
    parser.add_argument(
        "--port",
        default=7860,
        type=int,
        help="RPC serving port")
    args = parser.parse_args()

    handler = RPCHandler()
    handler.register_function(chat)

    models = []
    for _ in range(1):
        m = AutoModelForCausalLM.from_pretrained(args.model_name,
                                                 device_map="auto",
                                                 torch_dtype='auto')
        models.append(m)
    tokenizer = AutoTokenizer.from_pretrained(args.model_name)

    # Run the server
    rpc_server(handler, ('0.0.0.0', args.port),
               authkey=b'infiniflow-token4kevinhu')
