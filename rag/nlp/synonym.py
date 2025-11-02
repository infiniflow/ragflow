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
import json
import os
import time
import re
from nltk.corpus import wordnet
from common.file_utils import get_project_base_directory


class Dealer:
    def __init__(self, redis=None):

        self.lookup_num = 100000000
        self.load_tm = time.time() - 1000000
        self.dictionary = None
        path = os.path.join(get_project_base_directory(), "rag/res", "synonym.json")
        try:
            self.dictionary = json.load(open(path, 'r'))
            self.dictionary = { (k.lower() if isinstance(k, str) else k): v for k, v in self.dictionary.items() }
        except Exception:
            logging.warning("Missing synonym.json")
            self.dictionary = {}

        if not redis:
            logging.warning(
                "Realtime synonym is disabled, since no redis connection.")
        if not len(self.dictionary.keys()):
            logging.warning("Fail to load synonym")

        self.redis = redis
        self.load()

    def load(self):
        if not self.redis:
            return

        if self.lookup_num < 100:
            return
        tm = time.time()
        if tm - self.load_tm < 3600:
            return

        self.load_tm = time.time()
        self.lookup_num = 0
        d = self.redis.get("kevin_synonyms")
        if not d:
            return
        try:
            d = json.loads(d)
            self.dictionary = d
        except Exception as e:
            logging.error("Fail to load synonym!" + str(e))


    def lookup(self, tk, topn=8):
        if not tk or not isinstance(tk, str):
            return []

        # 1) Check the custom dictionary first (both keys and tk are already lowercase)
        self.lookup_num += 1
        self.load()
        key = re.sub(r"[ \t]+", " ", tk.strip())
        res = self.dictionary.get(key, [])
        if isinstance(res, str):
            res = [res]
        if res:  # Found in dictionary → return directly
            return res[:topn]

        # 2) If not found and tk is purely alphabetical → fallback to WordNet
        if re.fullmatch(r"[a-z]+", tk):
            wn_set = {
                re.sub("_", " ", syn.name().split(".")[0])
                for syn in wordnet.synsets(tk)
            }
            wn_set.discard(tk)  # Remove the original token itself
            wn_res = [t for t in wn_set if t]
            return wn_res[:topn]

        # 3) Nothing found in either source
        return []
    

if __name__ == '__main__':
    dl = Dealer()
    print(dl.dictionary)
