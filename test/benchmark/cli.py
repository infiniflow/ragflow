import argparse
import json
import os
import multiprocessing as mp
import time
from concurrent.futures import ProcessPoolExecutor, as_completed
from pathlib import Path
from typing import Any, Dict, List, Optional

from . import auth
from .auth import AuthError
from .chat import ChatError, create_chat, delete_chat, get_chat, resolve_model, stream_chat_completion
from .dataset import (
    DatasetError,
    create_dataset,
    dataset_has_chunks,
    delete_dataset,
    extract_document_ids,
    list_datasets,
    parse_documents,
    upload_documents,
    wait_for_parse_done,
)
from .http_client import HttpClient
from .metrics import ChatSample, RetrievalSample, summarize
from .report import chat_report, retrieval_report
from .retrieval import RetrievalError, build_payload, run_retrieval as run_retrieval_request
from .utils import eprint, load_json_arg, split_csv


def _parse_args() -> argparse.Namespace:
    base_parser = argparse.ArgumentParser(add_help=False)
    base_parser.add_argument(
        "--base-url",
        default=os.getenv("RAGFLOW_BASE_URL") or os.getenv("HOST_ADDRESS"),
        help="Base URL (env: RAGFLOW_BASE_URL or HOST_ADDRESS)",
    )
    base_parser.add_argument(
        "--api-version",
        default=os.getenv("RAGFLOW_API_VERSION", "v1"),
        help="API version (default: v1)",
    )
    base_parser.add_argument("--api-key", help="API key (Bearer token)")
    base_parser.add_argument("--connect-timeout", type=float, default=5.0, help="Connect timeout seconds")
    base_parser.add_argument("--read-timeout", type=float, default=60.0, help="Read timeout seconds")
    base_parser.add_argument("--no-verify-ssl", action="store_false", dest="verify_ssl", help="Disable SSL verification")
    base_parser.add_argument("--iterations", type=int, default=1, help="Number of iterations")
    base_parser.add_argument("--concurrency", type=int, default=1, help="Concurrency")
    base_parser.add_argument("--json", action="store_true", help="Print JSON report (optional)")
    base_parser.add_argument("--print-response", action="store_true", help="Print response content per iteration")
    base_parser.add_argument(
        "--response-max-chars",
        type=int,
        default=0,
        help="Truncate printed response to N chars (0 = no limit)",
    )

    # Auth/login options
    base_parser.add_argument("--login-email", default=os.getenv("RAGFLOW_EMAIL"), help="Login email")
    base_parser.add_argument("--login-nickname", default=os.getenv("RAGFLOW_NICKNAME"), help="Nickname for registration")
    base_parser.add_argument("--login-password", help="Login password (encrypted client-side)")
    base_parser.add_argument("--allow-register", action="store_true", help="Attempt /user/register before login")
    base_parser.add_argument("--token-name", help="Optional API token name")
    base_parser.add_argument("--bootstrap-llm", action="store_true", help="Ensure LLM factory API key is configured")
    base_parser.add_argument("--llm-factory", default=os.getenv("RAGFLOW_LLM_FACTORY"), help="LLM factory name")
    base_parser.add_argument("--llm-api-key", default=os.getenv("ZHIPU_AI_API_KEY"), help="LLM API key")
    base_parser.add_argument("--llm-api-base", default=os.getenv("RAGFLOW_LLM_API_BASE"), help="LLM API base URL")
    base_parser.add_argument("--set-tenant-info", action="store_true", help="Set tenant default model IDs")
    base_parser.add_argument("--tenant-llm-id", default=os.getenv("RAGFLOW_TENANT_LLM_ID"), help="Tenant chat model ID")
    base_parser.add_argument("--tenant-embd-id", default=os.getenv("RAGFLOW_TENANT_EMBD_ID"), help="Tenant embedding model ID")
    base_parser.add_argument("--tenant-img2txt-id", default=os.getenv("RAGFLOW_TENANT_IMG2TXT_ID"), help="Tenant image2text model ID")
    base_parser.add_argument("--tenant-asr-id", default=os.getenv("RAGFLOW_TENANT_ASR_ID", ""), help="Tenant ASR model ID")
    base_parser.add_argument("--tenant-tts-id", default=os.getenv("RAGFLOW_TENANT_TTS_ID"), help="Tenant TTS model ID")

    # Dataset/doc options
    base_parser.add_argument("--dataset-id", help="Existing dataset ID")
    base_parser.add_argument("--dataset-ids", help="Comma-separated dataset IDs")
    base_parser.add_argument("--dataset-name", default=os.getenv("RAGFLOW_DATASET_NAME"), help="Dataset name when creating")
    base_parser.add_argument("--dataset-payload", help="Dataset payload JSON or @file")
    base_parser.add_argument("--document-path", action="append", help="Document path (repeatable)")
    base_parser.add_argument("--document-paths-file", help="File with document paths, one per line")
    base_parser.add_argument("--parse-timeout", type=float, default=120.0, help="Parse timeout seconds")
    base_parser.add_argument("--parse-interval", type=float, default=1.0, help="Parse poll interval seconds")
    base_parser.add_argument("--teardown", action="store_true", help="Delete created resources after run")

    parser = argparse.ArgumentParser(description="RAGFlow HTTP API benchmark", parents=[base_parser])
    subparsers = parser.add_subparsers(dest="command", required=True)

    chat_parser = subparsers.add_parser(
        "chat",
        help="Chat streaming latency benchmark",
        parents=[base_parser],
        add_help=False,
    )
    chat_parser.add_argument("--chat-id", help="Existing chat ID")
    chat_parser.add_argument("--chat-name", default=os.getenv("RAGFLOW_CHAT_NAME"), help="Chat name when creating")
    chat_parser.add_argument("--chat-payload", help="Chat payload JSON or @file")
    chat_parser.add_argument("--model", default=os.getenv("RAGFLOW_CHAT_MODEL"), help="Model name for OpenAI endpoint")
    chat_parser.add_argument("--message", help="User message")
    chat_parser.add_argument("--messages-json", help="Messages JSON or @file")
    chat_parser.add_argument("--extra-body", help="extra_body JSON or @file")

    retrieval_parser = subparsers.add_parser(
        "retrieval",
        help="Retrieval latency benchmark",
        parents=[base_parser],
        add_help=False,
    )
    retrieval_parser.add_argument("--question", help="Retrieval question")
    retrieval_parser.add_argument("--payload", help="Retrieval payload JSON or @file")
    retrieval_parser.add_argument("--document-ids", help="Comma-separated document IDs")

    return parser.parse_args()


def _load_paths(args: argparse.Namespace) -> List[str]:
    paths = []
    if args.document_path:
        paths.extend(args.document_path)
    if args.document_paths_file:
        file_path = Path(args.document_paths_file)
        for line in file_path.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if line:
                paths.append(line)
    return paths


def _truncate_text(text: str, max_chars: int) -> str:
    if max_chars and len(text) > max_chars:
        return f"{text[:max_chars]}...[truncated]"
    return text


def _format_chat_response(sample: ChatSample, max_chars: int) -> str:
    if sample.error:
        text = f"[error] {sample.error}"
        if sample.response_text:
            text = f"{text} | {sample.response_text}"
    else:
        text = sample.response_text or ""
    if not text:
        text = "(empty)"
    return _truncate_text(text, max_chars)


def _format_retrieval_response(sample: RetrievalSample, max_chars: int) -> str:
    if sample.response is not None:
        text = json.dumps(sample.response, ensure_ascii=False, sort_keys=True)
        if sample.error:
            text = f"[error] {sample.error} | {text}"
    elif sample.error:
        text = f"[error] {sample.error}"
    else:
        text = "(empty)"
    return _truncate_text(text, max_chars)


def _chat_worker(
    base_url: str,
    api_version: str,
    api_key: str,
    connect_timeout: float,
    read_timeout: float,
    verify_ssl: bool,
    chat_id: str,
    model: str,
    messages: List[Dict[str, Any]],
    extra_body: Optional[Dict[str, Any]],
) -> ChatSample:
    client = HttpClient(
        base_url=base_url,
        api_version=api_version,
        api_key=api_key,
        connect_timeout=connect_timeout,
        read_timeout=read_timeout,
        verify_ssl=verify_ssl,
    )
    return stream_chat_completion(client, chat_id, model, messages, extra_body)


def _retrieval_worker(
    base_url: str,
    api_version: str,
    api_key: str,
    connect_timeout: float,
    read_timeout: float,
    verify_ssl: bool,
    payload: Dict[str, Any],
) -> RetrievalSample:
    client = HttpClient(
        base_url=base_url,
        api_version=api_version,
        api_key=api_key,
        connect_timeout=connect_timeout,
        read_timeout=read_timeout,
        verify_ssl=verify_ssl,
    )
    return run_retrieval_request(client, payload)


def _ensure_auth(client: HttpClient, args: argparse.Namespace) -> None:
    if args.api_key:
        client.api_key = args.api_key
        return
    if not args.login_email:
        raise AuthError("Missing API key and login email")
    if not args.login_password:
        raise AuthError("Missing login password")

    password_enc = auth.encrypt_password(args.login_password)

    if args.allow_register:
        nickname = args.login_nickname or args.login_email.split("@")[0]
        try:
            auth.register_user(client, args.login_email, nickname, password_enc)
        except AuthError as exc:
            eprint(f"Register warning: {exc}")

    login_token = auth.login_user(client, args.login_email, password_enc)
    client.login_token = login_token

    if args.bootstrap_llm:
        if not args.llm_factory:
            raise AuthError("Missing --llm-factory for bootstrap")
        if not args.llm_api_key:
            raise AuthError("Missing --llm-api-key for bootstrap")
        existing = auth.get_my_llms(client)
        if args.llm_factory not in existing:
            auth.set_llm_api_key(client, args.llm_factory, args.llm_api_key, args.llm_api_base)

    if args.set_tenant_info:
        if not args.tenant_llm_id or not args.tenant_embd_id:
            raise AuthError("Missing --tenant-llm-id or --tenant-embd-id for tenant setup")
        tenant = auth.get_tenant_info(client)
        tenant_id = tenant.get("tenant_id")
        if not tenant_id:
            raise AuthError("Tenant info missing tenant_id")
        payload = {
            "tenant_id": tenant_id,
            "llm_id": args.tenant_llm_id,
            "embd_id": args.tenant_embd_id,
            "img2txt_id": args.tenant_img2txt_id or "",
            "asr_id": args.tenant_asr_id or "",
            "tts_id": args.tenant_tts_id,
        }
        auth.set_tenant_info(client, payload)

    api_key = auth.create_api_token(client, login_token, args.token_name)
    client.api_key = api_key


def _prepare_dataset(
    client: HttpClient,
    args: argparse.Namespace,
    needs_dataset: bool,
    document_paths: List[str],
) -> Dict[str, Any]:
    created = {}
    dataset_ids = split_csv(args.dataset_ids) or []
    dataset_id = args.dataset_id
    dataset_payload = load_json_arg(args.dataset_payload, "dataset-payload") if args.dataset_payload else None

    if dataset_id:
        dataset_ids = [dataset_id]
    elif dataset_ids:
        dataset_id = dataset_ids[0]
    elif needs_dataset or document_paths:
        if not args.dataset_name and not (dataset_payload and dataset_payload.get("name")):
            raise DatasetError("Missing --dataset-name or dataset payload name")
        name = args.dataset_name or dataset_payload.get("name")
        data = create_dataset(client, name, dataset_payload)
        dataset_id = data.get("id")
        if not dataset_id:
            raise DatasetError("Dataset creation did not return id")
        dataset_ids = [dataset_id]
        created["Created Dataset ID"] = dataset_id
    return {
        "dataset_id": dataset_id,
        "dataset_ids": dataset_ids,
        "dataset_payload": dataset_payload,
        "created": created,
    }


def _maybe_upload_and_parse(
    client: HttpClient,
    dataset_id: str,
    document_paths: List[str],
    parse_timeout: float,
    parse_interval: float,
) -> List[str]:
    if not document_paths:
        return []
    docs = upload_documents(client, dataset_id, document_paths)
    doc_ids = extract_document_ids(docs)
    if not doc_ids:
        raise DatasetError("No document IDs returned after upload")
    parse_documents(client, dataset_id, doc_ids)
    wait_for_parse_done(client, dataset_id, doc_ids, parse_timeout, parse_interval)
    return doc_ids


def _ensure_dataset_has_chunks(client: HttpClient, dataset_id: str) -> None:
    datasets = list_datasets(client, dataset_id=dataset_id)
    if not datasets:
        raise DatasetError("Dataset not found")
    if not dataset_has_chunks(datasets[0]):
        raise DatasetError("Dataset has no parsed chunks; upload and parse documents first.")


def _cleanup(client: HttpClient, created: Dict[str, str], teardown: bool) -> None:
    if not teardown:
        return
    chat_id = created.get("Created Chat ID")
    if chat_id:
        try:
            delete_chat(client, chat_id)
        except Exception as exc:
            eprint(f"Cleanup warning: failed to delete chat {chat_id}: {exc}")
    dataset_id = created.get("Created Dataset ID")
    if dataset_id:
        try:
            delete_dataset(client, dataset_id)
        except Exception as exc:
            eprint(f"Cleanup warning: failed to delete dataset {dataset_id}: {exc}")


def run_chat(client: HttpClient, args: argparse.Namespace) -> int:
    document_paths = _load_paths(args)
    needs_dataset = bool(document_paths)
    dataset_info = _prepare_dataset(client, args, needs_dataset, document_paths)
    created = dict(dataset_info["created"])
    dataset_id = dataset_info["dataset_id"]
    dataset_ids = dataset_info["dataset_ids"]
    doc_ids = []
    if dataset_id and document_paths:
        doc_ids = _maybe_upload_and_parse(client, dataset_id, document_paths, args.parse_timeout, args.parse_interval)
        created["Created Document IDs"] = ",".join(doc_ids)
    if dataset_id and not document_paths:
        _ensure_dataset_has_chunks(client, dataset_id)
    if dataset_id and not document_paths and dataset_ids:
        _ensure_dataset_has_chunks(client, dataset_id)

    chat_payload = load_json_arg(args.chat_payload, "chat-payload") if args.chat_payload else None
    chat_id = args.chat_id
    if not chat_id:
        if not args.chat_name and not (chat_payload and chat_payload.get("name")):
            raise ChatError("Missing --chat-name or chat payload name")
        chat_name = args.chat_name or chat_payload.get("name")
        chat_data = create_chat(client, chat_name, dataset_ids or [], chat_payload)
        chat_id = chat_data.get("id")
        if not chat_id:
            raise ChatError("Chat creation did not return id")
        created["Created Chat ID"] = chat_id
    chat_data = get_chat(client, chat_id)
    model = resolve_model(args.model, chat_data)

    messages = None
    if args.messages_json:
        messages = load_json_arg(args.messages_json, "messages-json")
    if not messages:
        if not args.message:
            raise ChatError("Missing --message or --messages-json")
        messages = [{"role": "user", "content": args.message}]
    extra_body = load_json_arg(args.extra_body, "extra-body") if args.extra_body else None

    samples: List[ChatSample] = []
    responses: List[str] = []
    start_time = time.perf_counter()
    if args.concurrency <= 1:
        for _ in range(args.iterations):
            samples.append(stream_chat_completion(client, chat_id, model, messages, extra_body))
    else:
        results: List[Optional[ChatSample]] = [None] * args.iterations
        mp_context = mp.get_context("spawn")
        with ProcessPoolExecutor(max_workers=args.concurrency, mp_context=mp_context) as executor:
            future_map = {
                executor.submit(
                    _chat_worker,
                    client.base_url,
                    client.api_version,
                    client.api_key or "",
                    client.connect_timeout,
                    client.read_timeout,
                    client.verify_ssl,
                    chat_id,
                    model,
                    messages,
                    extra_body,
                ): idx
                for idx in range(args.iterations)
            }
            for future in as_completed(future_map):
                idx = future_map[future]
                results[idx] = future.result()
        samples = [sample for sample in results if sample is not None]
    total_duration = time.perf_counter() - start_time
    if args.print_response:
        for idx, sample in enumerate(samples, start=1):
            rendered = _format_chat_response(sample, args.response_max_chars)
            if args.json:
                responses.append(rendered)
            else:
                print(f"Response[{idx}]: {rendered}")

    total_latencies = [s.total_latency for s in samples if s.total_latency is not None and s.error is None]
    first_latencies = [s.first_token_latency for s in samples if s.first_token_latency is not None and s.error is None]
    success = len(total_latencies)
    failure = len(samples) - success
    errors = [s.error for s in samples if s.error]

    total_stats = summarize(total_latencies)
    first_stats = summarize(first_latencies)
    if args.json:
        payload = {
            "interface": "chat",
            "concurrency": args.concurrency,
            "iterations": args.iterations,
            "success": success,
            "failure": failure,
            "model": model,
            "total_latency": total_stats,
            "first_token_latency": first_stats,
            "errors": [e for e in errors if e],
            "created": created,
            "total_duration_s": total_duration,
            "qps": (args.iterations / total_duration) if total_duration > 0 else None,
        }
        if args.print_response:
            payload["responses"] = responses
        print(json.dumps(payload, sort_keys=True))
    else:
        report = chat_report(
            interface="chat",
            concurrency=args.concurrency,
            total_duration_s=total_duration,
            iterations=args.iterations,
            success=success,
            failure=failure,
            model=model,
            total_stats=total_stats,
            first_token_stats=first_stats,
            errors=[e for e in errors if e],
            created=created,
        )
        print(report, end="")
    _cleanup(client, created, args.teardown)
    return 0 if failure == 0 else 1


def run_retrieval(client: HttpClient, args: argparse.Namespace) -> int:
    document_paths = _load_paths(args)
    needs_dataset = True
    dataset_info = _prepare_dataset(client, args, needs_dataset, document_paths)
    created = dict(dataset_info["created"])
    dataset_id = dataset_info["dataset_id"]
    dataset_ids = dataset_info["dataset_ids"]
    if not dataset_ids:
        raise RetrievalError("dataset_ids required for retrieval")

    doc_ids = []
    if dataset_id and document_paths:
        doc_ids = _maybe_upload_and_parse(client, dataset_id, document_paths, args.parse_timeout, args.parse_interval)
        created["Created Document IDs"] = ",".join(doc_ids)

    payload_override = load_json_arg(args.payload, "payload") if args.payload else None
    question = args.question
    if not question and (payload_override is None or "question" not in payload_override):
        raise RetrievalError("Missing --question or retrieval payload question")
    document_ids = split_csv(args.document_ids) if args.document_ids else None

    payload = build_payload(question, dataset_ids, document_ids, payload_override)

    samples: List[RetrievalSample] = []
    responses: List[str] = []
    start_time = time.perf_counter()
    if args.concurrency <= 1:
        for _ in range(args.iterations):
            samples.append(run_retrieval_request(client, payload))
    else:
        results: List[Optional[RetrievalSample]] = [None] * args.iterations
        mp_context = mp.get_context("spawn")
        with ProcessPoolExecutor(max_workers=args.concurrency, mp_context=mp_context) as executor:
            future_map = {
                executor.submit(
                    _retrieval_worker,
                    client.base_url,
                    client.api_version,
                    client.api_key or "",
                    client.connect_timeout,
                    client.read_timeout,
                    client.verify_ssl,
                    payload,
                ): idx
                for idx in range(args.iterations)
            }
            for future in as_completed(future_map):
                idx = future_map[future]
                results[idx] = future.result()
        samples = [sample for sample in results if sample is not None]
    total_duration = time.perf_counter() - start_time
    if args.print_response:
        for idx, sample in enumerate(samples, start=1):
            rendered = _format_retrieval_response(sample, args.response_max_chars)
            if args.json:
                responses.append(rendered)
            else:
                print(f"Response[{idx}]: {rendered}")

    latencies = [s.latency for s in samples if s.latency is not None and s.error is None]
    success = len(latencies)
    failure = len(samples) - success
    errors = [s.error for s in samples if s.error]

    stats = summarize(latencies)
    if args.json:
        payload = {
            "interface": "retrieval",
            "concurrency": args.concurrency,
            "iterations": args.iterations,
            "success": success,
            "failure": failure,
            "latency": stats,
            "errors": [e for e in errors if e],
            "created": created,
            "total_duration_s": total_duration,
            "qps": (args.iterations / total_duration) if total_duration > 0 else None,
        }
        if args.print_response:
            payload["responses"] = responses
        print(json.dumps(payload, sort_keys=True))
    else:
        report = retrieval_report(
            interface="retrieval",
            concurrency=args.concurrency,
            total_duration_s=total_duration,
            iterations=args.iterations,
            success=success,
            failure=failure,
            stats=stats,
            errors=[e for e in errors if e],
            created=created,
        )
        print(report, end="")
    _cleanup(client, created, args.teardown)
    return 0 if failure == 0 else 1


def main() -> None:
    args = _parse_args()
    if not args.base_url:
        raise SystemExit("Missing --base-url or HOST_ADDRESS")
    if args.iterations < 1:
        raise SystemExit("--iterations must be >= 1")
    if args.concurrency < 1:
        raise SystemExit("--concurrency must be >= 1")
    client = HttpClient(
        base_url=args.base_url,
        api_version=args.api_version,
        api_key=args.api_key,
        connect_timeout=args.connect_timeout,
        read_timeout=args.read_timeout,
        verify_ssl=args.verify_ssl,
    )
    try:
        _ensure_auth(client, args)
        if args.command == "chat":
            raise SystemExit(run_chat(client, args))
        if args.command == "retrieval":
            raise SystemExit(run_retrieval(client, args))
        raise SystemExit("Unknown command")
    except (AuthError, DatasetError, ChatError, RetrievalError) as exc:
        eprint(f"Error: {exc}")
        raise SystemExit(2)
