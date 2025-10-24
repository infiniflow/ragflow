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


import hypothesis.strategies as st


@st.composite
def valid_names(draw):
    base_chars = "abcdefghijklmnopqrstuvwxyz_"
    first_char = draw(st.sampled_from([c for c in base_chars if c.isalpha() or c == "_"]))
    remaining = draw(st.text(alphabet=st.sampled_from(base_chars), min_size=0, max_size=128 - 2))

    name = (first_char + remaining)[:128]
    return name.encode("utf-8").decode("utf-8")
