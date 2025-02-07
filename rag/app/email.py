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

import logging
from email import policy
from email.parser import BytesParser
from rag.app.naive import chunk as naive_chunk
import re
from rag.nlp import rag_tokenizer, naive_merge, tokenize_chunks
from deepdoc.parser import HtmlParser, TxtParser
from timeit import default_timer as timer
import io


def chunk(
    filename,
    binary=None,
    from_page=0,
    to_page=100000,
    lang="Chinese",
    callback=None,
    **kwargs,
):
    """
    Only eml is supported
    """
    eng = lang.lower() == "english"  # is_english(cks)
    parser_config = kwargs.get(
        "parser_config",
        {"chunk_token_num": 128, "delimiter": "\n!?。；！？", "layout_recognize": "DeepDOC"},
    )
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    main_res = []
    attachment_res = []

    if binary:
        msg = BytesParser(policy=policy.default).parse(io.BytesIO(binary))
    else:
        msg = BytesParser(policy=policy.default).parse(open(filename, "rb"))

    text_txt, html_txt = [], []
    # get the email header info
    for header, value in msg.items():
        text_txt.append(f"{header}: {value}")

    #  get the email main info
    def _add_content(msg, content_type):
        if content_type == "text/plain":
            text_txt.append(
                msg.get_payload(decode=True).decode(msg.get_content_charset())
            )
        elif content_type == "text/html":
            html_txt.append(
                msg.get_payload(decode=True).decode(msg.get_content_charset())
            )
        elif "multipart" in content_type:
            if msg.is_multipart():
                for part in msg.iter_parts():
                    _add_content(part, part.get_content_type())

    _add_content(msg, msg.get_content_type())

    sections = TxtParser.parser_txt("\n".join(text_txt)) + [
        (line, "") for line in HtmlParser.parser_txt("\n".join(html_txt)) if line
    ]

    st = timer()
    chunks = naive_merge(
        sections,
        int(parser_config.get("chunk_token_num", 128)),
        parser_config.get("delimiter", "\n!?。；！？"),
    )

    main_res.extend(tokenize_chunks(chunks, doc, eng, None))
    logging.debug("naive_merge({}): {}".format(filename, timer() - st))
    # get the attachment info
    for part in msg.iter_attachments():
        content_disposition = part.get("Content-Disposition")
        if content_disposition:
            dispositions = content_disposition.strip().split(";")
            if dispositions[0].lower() == "attachment":
                filename = part.get_filename()
                payload = part.get_payload(decode=True)
                try:
                    attachment_res.extend(
                        naive_chunk(filename, payload, callback=callback, **kwargs)
                    )
                except Exception:
                    pass

    return main_res + attachment_res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], callback=dummy)
