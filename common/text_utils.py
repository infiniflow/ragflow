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

from __future__ import annotations


def normalize_arabic_digits(text: str | None) -> str | None:
    if text is None or not isinstance(text, str):
        return text

    out = []
    for ch in text:
        code = ord(ch)
        if 0x0660 <= code <= 0x0669:
            out.append(chr(code - 0x0660 + 0x30))
        elif 0x06F0 <= code <= 0x06F9:
            out.append(chr(code - 0x06F0 + 0x30))
        else:
            out.append(ch)
    return "".join(out)
