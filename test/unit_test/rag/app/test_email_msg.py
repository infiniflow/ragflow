"""Regression tests for Outlook .msg conversion inside the RAGFlow email parser."""

import importlib.util
from email.message import EmailMessage
from pathlib import Path
import subprocess
import sys
import io
import shutil
import struct
from types import SimpleNamespace
from unittest.mock import MagicMock, Mock

import pytest


OLE = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
BINARY_MSG = OLE + b"binary content must never become searchable text"
PDF = b"%PDF-1.4\n\x00\xff attachment bytes"


@pytest.fixture
def parser(monkeypatch):
    """Import the email parser with dependencies mocked for unit tests."""

    def tokenize(text):
        return str(text)

    text_parser = SimpleNamespace(parser_txt=lambda text: [(text, "")] if text else [])
    html_parser = SimpleNamespace(parser_txt=lambda text, **kwargs: [text] if text else [])
    naive = Mock(return_value=[{"content_with_weight": "Parsed attachment"}])
    nlp = SimpleNamespace(
        rag_tokenizer=SimpleNamespace(tokenize=tokenize, fine_grained_tokenize=tokenize),
        naive_merge=lambda sections, *args: ["\n".join(section[0] for section in sections)] if sections else [],
        tokenize_chunks=lambda chunks, doc, *args, **kwargs: [dict(doc, content_with_weight=text) for text in chunks],
        DEFAULT_DELIMITER="\n",
    )
    reader = MagicMock()
    reader.return_value.__enter__.return_value.body = None
    reader.return_value.__enter__.return_value.getStream.return_value = None
    with monkeypatch.context() as deps:
        deps.setitem(sys.modules, "extract_msg", SimpleNamespace(Message=reader))
        deps.setitem(sys.modules, "rag.app.naive", SimpleNamespace(chunk=naive))
        deps.setitem(sys.modules, "common.constants", SimpleNamespace(MAXIMUM_PAGE_NUMBER=1000000))
        deps.setitem(sys.modules, "rag.nlp", nlp)
        deps.setitem(sys.modules, "deepdoc.parser", SimpleNamespace(HtmlParser=html_parser, TxtParser=text_parser))
        path = Path(__file__).resolve().parents[4] / "rag/app/email.py"
        spec = importlib.util.spec_from_file_location("email_parser_under_test", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    return module


def eml(attachment=False, html=False, body=True):
    """Build a test RFC822 message with optional plain body, HTML and attachment."""
    message = EmailMessage()
    message["From"] = "Projektant <sender@example.test>"
    message["To"] = "receiver@example.test"
    message["Cc"] = "copy@example.test"
    message["Reply-To"] = "reply@example.test"
    message["Date"] = "Tue, 15 Sep 2026 10:00:00 +0200"
    message["Subject"] = "Žiadosť o zmenu projektu"
    message["Received"] = "transport-noise"
    message["DKIM-Signature"] = "signature-noise"
    message["X-Microsoft-Antispam-Message-Info"] = "antispam-noise"
    message["Thread-Index"] = "thread-noise"
    if body:
        message.set_content("Dohodnutý termín odovzdania: štvrtok.")
    if html:
        message.add_alternative("<p>Žiadosť v HTML</p>", subtype="html")
    if attachment:
        message.add_attachment(PDF, maintype="application", subtype="pdf", filename="príloha.pdf")
    return message.as_bytes()


def convert_with(monkeypatch, output):
    """Return a mock subprocess.run that writes output.eml and records temp dirs."""
    directories = []

    def run(command, **kwargs):
        assert command[:2] == ["msgconvert", "--outfile"]
        destination, source = Path(command[2]), Path(command[3])
        assert source.read_bytes() == BINARY_MSG
        assert source.parent == destination.parent
        assert source.name == "input.msg"
        assert source.parent.stat().st_mode & 0o077 == 0
        assert kwargs["cwd"] == str(source.parent)
        assert kwargs["check"] is True
        assert kwargs["timeout"] == 300
        assert kwargs["stdout"] == subprocess.DEVNULL
        assert kwargs["stderr"] == subprocess.DEVNULL
        assert not kwargs.get("shell")
        directories.append(source.parent)
        destination.write_bytes(output)
        return subprocess.CompletedProcess(command, 0)

    mock = Mock(side_effect=run)
    monkeypatch.setattr(subprocess, "run", mock)
    return mock, directories


def text(chunks):
    """Concatenate chunk text for easy assertion checks."""
    return "\n".join(chunk["content_with_weight"] for chunk in chunks)


@pytest.mark.parametrize("name", ["message.msg", "message.MSG", "renamed.eml", "folder/$(touch injected).msg"])
def test_msg_conversion_keeps_display_name_and_readable_content(parser, monkeypatch, name):
    """MSG conversion preserves filename and yields readable Slovak text."""
    converter, directories = convert_with(monkeypatch, eml())
    chunks = parser.chunk(name, binary=BINARY_MSG, lang="Slovak")
    converter.assert_called_once()
    assert "Žiadosť o zmenu projektu" in text(chunks)
    assert "Dohodnutý termín" in text(chunks)
    assert "binary content" not in text(chunks)
    assert all(chunk["docnm_kwd"] == name for chunk in chunks)
    assert all(not directory.exists() for directory in directories)


def test_msg_file_input_keeps_original_unchanged(parser, monkeypatch, tmp_path):
    """Reading from a file path leaves the original .msg file untouched."""
    source = tmp_path / "original.msg"
    source.write_bytes(BINARY_MSG)
    convert_with(monkeypatch, eml())
    assert "Dohodnutý termín" in text(parser.chunk(str(source), lang="Slovak"))
    assert source.read_bytes() == BINARY_MSG


@pytest.mark.parametrize("convert", [False, True])
def test_only_searchable_headers_are_indexed(parser, monkeypatch, convert):
    """Only relevant email headers are indexed; transport noise is dropped."""
    if convert:
        convert_with(monkeypatch, eml())
    chunks = parser.chunk("mail.msg" if convert else "mail.eml", binary=BINARY_MSG if convert else eml(), lang="Slovak")
    result = text(chunks)
    for value in ["sender@example.test", "receiver@example.test", "copy@example.test", "reply@example.test", "15 Sep 2026", "Žiadosť"]:
        assert value in result
    for value in ["transport-noise", "signature-noise", "antispam-noise", "thread-noise", "MIME-Version:", "Content-Type:"]:
        assert value not in result


@pytest.mark.parametrize("convert", [False, True])
def test_pdf_attachment_keeps_bytes_language_and_parser_config(parser, monkeypatch, convert):
    """Attachments keep filename, binary payload, language and parser config."""
    raw = eml(attachment=True, html=True)
    if convert:
        convert_with(monkeypatch, raw)
    config = {"chunk_token_num": 512, "layout_recognize": "DeepDOC"}
    callback = Mock()
    chunks = parser.chunk("mail.msg" if convert else "mail.eml", binary=BINARY_MSG if convert else raw, lang="Slovak", callback=callback, parser_config=config)
    parser.naive_chunk.assert_called_once_with("príloha.pdf", PDF, lang="Slovak", callback=callback, parser_config=config)
    assert "Parsed attachment" in text(chunks)
    assert "Žiadosť v HTML" in text(chunks)


@pytest.mark.parametrize(
    "error",
    [FileNotFoundError("private diagnostic"), subprocess.CalledProcessError(1, "msgconvert", stderr=b"private diagnostic"), subprocess.TimeoutExpired("msgconvert", 300, output=b"private diagnostic")],
)
def test_conversion_failure_never_indexes_binary_and_cleans_staging(parser, monkeypatch, error):
    """Conversion failures surface as RuntimeError and clean up staging directories."""
    directories = []

    def fail(command, **kwargs):
        directories.append(Path(command[3]).parent)
        raise error

    monkeypatch.setattr(subprocess, "run", fail)
    with pytest.raises(RuntimeError, match="MSG conversion failed") as exc:
        parser.chunk("private display name.msg", binary=BINARY_MSG)
    assert "private" not in str(exc.value)
    assert directories and all(not directory.exists() for directory in directories)


@pytest.mark.parametrize("output", [b"", b"not an email", BINARY_MSG, b"Subject: broken\nContent-Type: multipart/mixed; boundary=missing\n\ninvalid"])
def test_invalid_conversion_output_is_rejected(parser, monkeypatch, output):
    """Malformed conversion output is rejected before chunking."""
    _, directories = convert_with(monkeypatch, output)
    with pytest.raises(ValueError, match="Invalid converted email"):
        parser.chunk("mail.msg", binary=BINARY_MSG)
    assert all(not directory.exists() for directory in directories)


@pytest.mark.parametrize("raw", [b"", b"not an Outlook file", OLE[:4]])
def test_invalid_msg_signature_is_rejected(parser, monkeypatch, raw):
    """MSG input without the OLE signature is rejected before conversion."""
    converter = Mock()
    monkeypatch.setattr(subprocess, "run", converter)
    with pytest.raises(ValueError, match="Invalid Outlook MSG signature"):
        parser.chunk("mail.msg", binary=raw)
    converter.assert_not_called()


def test_eml_does_not_need_converter_and_empty_binary_does_not_open_filename(parser, monkeypatch):
    """Plain .eml files bypass MSG conversion and empty input returns no chunks."""
    converter = Mock(side_effect=AssertionError("must not convert EML"))
    monkeypatch.setattr(subprocess, "run", converter)
    assert "Dohodnutý termín" in text(parser.chunk("mail.eml", binary=eml()))
    assert parser.chunk("nonexistent.eml", binary=b"") == []
    converter.assert_not_called()


def test_msg_plain_body_comes_from_source_not_converter(parser, monkeypatch):
    """Source plain-text body overrides msgconvert output when it differs."""
    convert_with(monkeypatch, eml(attachment=True))
    parser.OutlookMessage.return_value.__enter__.return_value.body = "Správny pôvodný text: ľščťžýáíé."
    chunks = parser.chunk("mail.msg", binary=BINARY_MSG, lang="Slovak")
    assert "Správny pôvodný text: ľščťžýáíé." in text(chunks)
    assert "Dohodnutý termín" not in text(chunks)
    assert parser.OutlookMessage.call_args.kwargs == {"delayAttachments": True}
    parser.OutlookMessage.return_value.__exit__.assert_called_once()
    assert parser.naive_chunk.call_args.args == ("príloha.pdf", PDF)


@pytest.mark.parametrize("charset", ["utf-8", "windows-1250"])
def test_msg_html_body_keeps_source_bytes_and_declared_charset(parser, monkeypatch, charset):
    """Source HTML bytes and declared charset override msgconvert HTML output."""
    message = EmailMessage()
    message["Subject"] = "HTML message"
    message.set_content("Incorrect converted HTML", subtype="html", charset=charset)
    convert_with(monkeypatch, message.as_bytes())
    raw_html = "<p>Pôvodný text: ľščťžýáíé.</p>".encode(charset)
    reader = parser.OutlookMessage.return_value.__enter__.return_value
    reader.getStream.return_value = raw_html
    parsed = parser._parse_message("mail.msg", BINARY_MSG)
    assert parsed.get_payload(decode=True) == raw_html
    assert parsed.get_content_charset() == charset
    assert "Pôvodný text" in parsed.get_content()
    reader.getStream.assert_called_once_with("__substg1.0_10130102")


def test_source_reader_failure_does_not_silently_use_converted_body(parser, monkeypatch):
    """extract_msg failures are treated as conversion failures, not silent fallback."""
    _, directories = convert_with(monkeypatch, eml())
    parser.OutlookMessage.side_effect = ValueError("private source diagnostic")
    with pytest.raises(RuntimeError, match="MSG conversion failed") as exc:
        parser.chunk("mail.msg", binary=BINARY_MSG)
    assert "private" not in str(exc.value)
    assert all(not directory.exists() for directory in directories)


@pytest.mark.skipif(shutil.which("msgconvert") is None, reason="msgconvert system package required")
def test_real_msgconvert_preserves_unicode_body_and_pdf_attachment(parser, monkeypatch):
    """Real msgconvert run keeps Slovak diacritics, HTML body and PDF attachment."""
    monkeypatch.setattr(parser, "OutlookMessage", pytest.importorskip("extract_msg").Message)
    writer_class = pytest.importorskip("extract_msg.ole_writer").OleWriter
    writer = writer_class()
    for tag in ("00020102", "00030102", "00040102"):
        writer.addEntry("__nameid_version1.0/__substg1.0_" + tag, b"")
    writer.addEntry("__properties_version1.0", bytes(32) + struct.pack("<IIII", 0x3FDE0003, 6, 65001, 0) + struct.pack("<IIII", 0x340D0003, 6, 0x40000, 0))
    properties = {
        "0037001F": "Žiadosť o zmenu projektu",
        "1000001F": "Dohodnutý termín odovzdania: štvrtok.",
        "0C1A001F": "Projektant",
        "0C1F001F": "sender@example.test",
        "007D001F": "From: sender@example.test\r\nTo: receiver@example.test\r\nDate: Tue, 15 Sep 2026 10:00:00 +0200\r\nReceived: transport-noise\r\n",
    }
    for tag, value in properties.items():
        writer.addEntry("__substg1.0_" + tag, (value + "\0").encode("utf-16le"))
    writer.addEntry("__substg1.0_10130102", '<html><meta charset="utf-8"><p>Žiadosť v HTML</p></html>'.encode("utf-8"))
    attachment = "__attach_version1.0_#00000000/"
    writer.addEntry(attachment + "__properties_version1.0", bytes(8) + struct.pack("<IIII", 0x37050003, 6, 1, 0))
    writer.addEntry(attachment + "__substg1.0_37010102", PDF)
    for tag, value in {"3707001F": "príloha.pdf", "3704001F": "príloha.pdf", "370E001F": "application/pdf"}.items():
        writer.addEntry(attachment + "__substg1.0_" + tag, (value + "\0").encode("utf-16le"))
    output = io.BytesIO()
    writer.write(output)
    raw = output.getvalue()
    assert raw.startswith(OLE)
    with pytest.importorskip("extract_msg").Message(raw) as original:
        assert "Dohodnutý termín" in original.body
    message = parser._parse_message("synthetic.msg", raw)
    assert "Žiadosť" in str(message["Subject"])
    assert "Dohodnutý termín" in message.get_body(preferencelist=("plain",)).get_content()
    assert "Žiadosť v HTML" in message.get_body(preferencelist=("html",)).get_content()
    attachments = list(message.iter_attachments())
    assert len(attachments) == 1
    assert attachments[0].get_filename() == "príloha.pdf"
    assert attachments[0].get_payload(decode=True) == PDF
    chunks = parser.chunk("synthetic.msg", binary=raw, lang="Slovak")
    assert "transport-noise" not in text(chunks)
    assert "Dohodnutý termín" in text(chunks)
    assert parser.naive_chunk.call_args.args == ("príloha.pdf", PDF)


def test_missing_plain_body_is_inserted_from_source(parser, monkeypatch):
    """When msgconvert omits the plain body, the source body is added to the parsed message."""
    convert_with(monkeypatch, eml(body=False, html=False))
    reader = parser.OutlookMessage.return_value.__enter__.return_value
    reader.body = "Zdrojové telo s diakritikou: šťastie."
    reader.getStream.return_value = None
    message = parser._parse_message("mail.msg", BINARY_MSG)
    assert "Zdrojové telo s diakritikou" in message.get_body(preferencelist=("plain",)).get_content()


def test_missing_html_body_is_inserted_from_source(parser, monkeypatch):
    """When msgconvert omits the HTML alternative, the source HTML is added to the parsed message."""
    convert_with(monkeypatch, eml(body=True, html=False))
    reader = parser.OutlookMessage.return_value.__enter__.return_value
    reader.getStream.return_value = "<html><p>HTML telo: šťastie.</p></html>".encode("utf-8")
    message = parser._parse_message("mail.msg", BINARY_MSG)
    html_part = message.get_body(preferencelist=("html",))
    assert html_part is not None
    assert "HTML telo: šťastie" in html_part.get_content()


def test_missing_both_bodies_are_inserted_from_source(parser, monkeypatch):
    """When msgconvert omits both body alternatives, both are restored from the source message."""
    convert_with(monkeypatch, eml(body=False, html=False))
    reader = parser.OutlookMessage.return_value.__enter__.return_value
    reader.body = "Zdrojové telo."
    reader.getStream.return_value = b"<html><p>HTML telo.</p></html>"
    chunks = parser.chunk("mail.msg", binary=BINARY_MSG, lang="Slovak")
    assert "Zdrojové telo." in text(chunks)
    assert "HTML telo." in text(chunks)
    assert "binary content" not in text(chunks)
