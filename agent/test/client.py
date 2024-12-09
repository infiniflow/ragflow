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
import argparse
import os
from functools import partial
from agent.canvas import Canvas
from agent.settings import DEBUG

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    dsl_default_path = os.path.join(
        os.path.dirname(os.path.realpath(__file__)),
        "dsl_examples",
        "retrieval_and_generate.json",
    )
    parser.add_argument('-s', '--dsl', default=dsl_default_path, help="input dsl", action='store', required=True)
    parser.add_argument('-t', '--tenant_id', default=False, help="Tenant ID", action='store', required=True)
    parser.add_argument('-m', '--stream', default=False, help="Stream output", action='store_true', required=False)
    args = parser.parse_args()

    canvas = Canvas(open(args.dsl, "r").read(), args.tenant_id)
    while True:
        ans = canvas.run(stream=args.stream)
        print("==================== Bot =====================\n>    ", end='')
        if args.stream and isinstance(ans, partial):
            cont = ""
            for an in ans():
                print(an["content"][len(cont):], end='', flush=True)
                cont = an["content"]
        else:
            print(ans["content"])

        if DEBUG:
            print(canvas.path)
        question = input("\n==================== User =====================\n> ")
        canvas.add_user_input(question)
