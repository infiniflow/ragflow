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
from email.message import EmailMessage as _EmailMessage
from email.parser import BytesParser
from rag.app.naive import chunk as naive_chunk
from common.constants import MAXIMUM_PAGE_NUMBER
import re
from rag.nlp import rag_tokenizer, naive_merge, tokenize_chunks, DEFAULT_DELIMITER
from deepdoc.parser import HtmlParser, TxtParser
from timeit import default_timer as timer
from pathlib import Path
import subprocess
import tempfile
from extract_msg import Message as OutlookMessage


_OLE_SIGNATURE = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
_INDEXED_HEADERS = frozenset(("date", "from", "to", "cc", "bcc", "reply-to", "subject"))


def _replace_or_add_body(message, body, body_type):
    """Replace an existing plain/html body part or add it if msgconvert omitted it."""
    if body is None:
        return
    part = message.get_body(preferencelist=(body_type,))
    if part is not None:
        if body_type == "plain":
            part.set_content(body)
        else:
            charset = part.get_content_charset() or "utf-8"
            part.set_content(body, maintype="text", subtype="html", params={"charset": charset})
        return
    new_part = _EmailMessage()
    if body_type == "plain":
        new_part.set_content(body)
    else:
        new_part.set_content(body, maintype="text", subtype="html", params={"charset": "utf-8"})
    if message.is_multipart():
        message.attach(new_part)
    elif message.get_content_type() == "text/plain" and body_type == "html":
        message.make_alternative()
        message.attach(new_part)
    elif message.get_content_type() == "text/html" and body_type == "plain":
        message.make_alternative()
        message.attach(new_part)
    elif body_type == "plain":
        message.set_content(body)
    else:
        message.set_content(body, maintype="text", subtype="html", params={"charset": "utf-8"})


def _parse_message(filename, binary):
    """Parse an RFC822 email; convert Outlook MSG to EML first when necessary."""
    original_body = None
    original_html = None
    is_msg = filename.lower().endswith(".msg") or binary.startswith(_OLE_SIGNATURE)
    if is_msg:
        if not binary.startswith(_OLE_SIGNATURE):
            raise ValueError("Invalid Outlook MSG signature")
        try:
            with tempfile.TemporaryDirectory(prefix="ragflow-msg-") as directory:
                source = Path(directory) / "input.msg"
                destination = Path(directory) / "output.eml"
                source.write_bytes(binary)
                subprocess.run(
                    ["msgconvert", "--outfile", str(destination), str(source)],
                    cwd=directory,
                    check=True,
                    timeout=300,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                )
                binary = destination.read_bytes()
                with OutlookMessage(str(source), delayAttachments=True) as original:
                    original_body = original.body
                    original_html = original.getStream("__substg1.0_10130102")
        except Exception:
            raise RuntimeError("MSG conversion failed; verify msgconvert is installed and the Outlook file is valid") from None
    message = BytesParser(policy=policy.default).parsebytes(binary)
    if is_msg and (message.defects or not any(header.lower() in _INDEXED_HEADERS for header in message.keys())):
        raise ValueError("Invalid converted email")
    _replace_or_add_body(message, original_body, "plain")
    _replace_or_add_body(message, original_html, "html")
    return message


def chunk(
    filename,
    binary=None,
    from_page=0,
    to_page=MAXIMUM_PAGE_NUMBER,
    lang="Chinese",
    callback=None,
    **kwargs,
):
    """
    RFC822 emails and Outlook MSG files are supported
    """
    eng = lang.lower() == "english"  # is_english(cks)
    parser_config = kwargs.get(
        "parser_config",
        {"chunk_token_num": 512, "delimiter": DEFAULT_DELIMITER, "layout_recognize": "DeepDOC"},
    )
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    main_res = []
    attachment_res = []

    if binary is None:
        with open(filename, "rb") as buffer:
            binary = buffer.read()
    msg = _parse_message(filename, binary)

    text_txt, html_txt = [], []
    # get the email header info
    for header, value in msg.items():
        if header.lower() in _INDEXED_HEADERS:
            text_txt.append(f"{header}: {value}")

    #  get the email main info
    def _add_content(msg, content_type):
        """Recursively collect text/plain and text/html payloads into target lists."""

        def _decode_payload(payload, charset, target_list):
            """Decode a MIME payload, falling back through common encodings."""
            try:
                target_list.append(payload.decode(charset))
            except (UnicodeDecodeError, LookupError):
                for enc in ["utf-8", "gb2312", "gbk", "gb18030", "latin1"]:
                    try:
                        target_list.append(payload.decode(enc))
                        break
                    except UnicodeDecodeError:
                        continue
                else:
                    target_list.append(payload.decode("utf-8", errors="ignore"))

        if content_type == "text/plain":
            payload = msg.get_payload(decode=True)
            charset = msg.get_content_charset() or "utf-8"
            _decode_payload(payload, charset, text_txt)
        elif content_type == "text/html":
            payload = msg.get_payload(decode=True)
            charset = msg.get_content_charset() or "utf-8"
            _decode_payload(payload, charset, html_txt)
        elif "multipart" in content_type:
            if msg.is_multipart():
                for part in msg.iter_parts():
                    _add_content(part, part.get_content_type())

    _add_content(msg, msg.get_content_type())

    sections = TxtParser.parser_txt("\n".join(text_txt)) + [(line, "") for line in HtmlParser.parser_txt("\n".join(html_txt), chunk_token_num=parser_config["chunk_token_num"]) if line]

    st = timer()
    chunks = naive_merge(
        sections,
        int(parser_config.get("chunk_token_num", 128)),
        parser_config.get("delimiter", DEFAULT_DELIMITER),
    )

    main_res.extend(tokenize_chunks(chunks, doc, eng, None, language=lang))
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
                    attachment_res.extend(naive_chunk(filename, payload, lang=lang, callback=callback, **kwargs))
                except Exception:
                    pass

    return main_res + attachment_res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], callback=dummy)
