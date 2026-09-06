"""Disposable live T1 runner; generated secrets remain in the process environment."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import time
from urllib.request import Request, urlopen

import yaml

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
PROJECT = "ragflow-t1-live-20260906-b-" + secrets.token_hex(4)
BASE = "http://127.0.0.1:19382"
SECRET_VALUES = []
MODELS = (("qwen2.5:7b-instruct", "chat"), ("llama3.1:8b-instruct-q4_K_M", "chat"), ("bge-m3:latest", "embedding"))


def sanitized(text):
    for value in SECRET_VALUES:
        if value:
            text = text.replace(value, "[REDACTED]")
    return text


def command(args, *, env, log, input=None, check=True, cwd=None, timeout=180):
    process = subprocess.Popen(
        args,
        env=env,
        cwd=cwd or ROOT,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
        start_new_session=os.name != "nt",
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    )
    timed_out = False
    try:
        stdout, stderr = process.communicate(input=input, timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        if os.name == "nt":
            subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], capture_output=True, timeout=30, check=False)
        else:
            os.killpg(process.pid, signal.SIGKILL)
        stdout, stderr = process.communicate(timeout=30)
    result = subprocess.CompletedProcess(args, 124 if timed_out else process.returncode, stdout, stderr)
    (OUT / log).write_text(sanitized((stdout or "") + (stderr or "") + ("\nCommand timeout\n" if timed_out else "")), encoding="utf-8")
    with (OUT / "commands.jsonl").open("a", encoding="utf-8") as stream:
        stream.write(sanitized(json.dumps({"argv": args, "cwd": str(cwd or ROOT), "timeout_seconds": timeout, "exit_code": result.returncode, "log": log})) + "\n")
    if check and result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}); see {log}")
    return result


def api(path, method="GET", payload=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = token
    with urlopen(Request(BASE + path, method=method, headers=headers, data=json.dumps(payload).encode() if payload is not None else None), timeout=240) as response:
        result = json.load(response)
        assert result.get("code") == 0, f"{method} {path}: code={result.get('code')} message={result.get('message')}"
        return result.get("data"), response.headers.get("Authorization")


def verify_source(candidate, source, env, phase):
    script = "import sys,json;sys.path.insert(0,sys.argv[1]);from candidate import verify;m=verify(__import__('pathlib').Path(sys.argv[2]));print(json.dumps({'source_id':m['source_id'],'integrity':'PASS'}))"
    command([sys.executable, "-B", "-c", script, str(source / "tools/quality"), str(candidate)], env=env, log=f"candidate-{phase}.log")


def main():
    global OUT
    sys.dont_write_bytecode = True
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--dist-root", type=Path, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--frontend-archive", type=Path, help="Verified frontend archive and adjacent .json receipt; required for frozen candidates")
    parser.add_argument("tests", nargs="*")
    args = parser.parse_args()
    OUT = args.evidence_dir.resolve()
    OUT.mkdir(parents=True, exist_ok=True)
    source = args.source_root.resolve()
    dist = args.dist_root.resolve()
    assert (source / "api/ragflow_server.py").is_file()
    assert (dist / "index.html").is_file()
    for port in (19382, 19383):
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", port))
    env = dict(os.environ)
    env.update(PYTHONUTF8="1", PYTHONIOENCODING="utf-8")
    env.update(QA_STORAGE_PASSWORD=secrets.token_urlsafe(32), QA_ADMIN_PASSWORD=secrets.token_urlsafe(32), QA_ADMIN_EMAIL=f"regression-probe-{secrets.token_hex(5)}@example.org")
    SECRET_VALUES.extend([env["QA_STORAGE_PASSWORD"], env["QA_ADMIN_PASSWORD"]])
    for resource, arguments in (("containers", ["ps", "-a"]), ("volumes", ["volume", "ls"]), ("networks", ["network", "ls"])):
        existing = command(["docker", *arguments, "-q", "--filter", f"label=com.docker.compose.project={PROJECT}"], env=env, log=f"preflight-{resource}.log")
        assert not existing.stdout.strip(), "Disposable project name is already in use"
    compose = yaml.safe_load(Path(__file__).with_name("compose.yml").read_text(encoding="utf-8"))
    compose["name"] = PROJECT
    app = compose["services"]["app"]
    app["ports"] = ["127.0.0.1:19382:80", "127.0.0.1:19383:9381"]
    app["command"].append("--workers=1")
    app["environment"].update(
        DEFAULT_SUPERUSER_EMAIL="${QA_ADMIN_EMAIL:?required}", DEFAULT_SUPERUSER_PASSWORD="${QA_ADMIN_PASSWORD:?required}", MANAGED_RESOURCE_OWNER_EMAIL="${QA_ADMIN_EMAIL:?required}"
    )
    app["volumes"] = [f"{source.as_posix()}/{item[6:]}" if item.startswith("../../") else item for item in app["volumes"]]
    app["volumes"] = [f"{dist.as_posix()}:/ragflow/web/dist:ro" if ":/ragflow/web/dist:" in item else item for item in app["volumes"]]
    candidate = source.parent if (source.parent / "candidate.json").is_file() else None
    assert not candidate or not OUT.is_relative_to(source), "Evidence must be outside the frozen source snapshot"
    if candidate:
        input_files = json.loads((candidate / "candidate.json").read_text(encoding="utf-8"))["identity"]["files"]
    else:
        input_files = subprocess.check_output(["git", "ls-files", "conf", "rag/res"], cwd=source, text=True).splitlines()
    mounted_assets = {}
    for name in input_files:
        if (name.startswith("conf/") and name != "conf/service_conf.yaml") or name.startswith("rag/res/"):
            app["volumes"].append(f"{(source / name).as_posix()}:/ragflow/{name}:ro")
            mounted_assets["/ragflow/" + name] = hashlib.sha256((source / name).read_bytes()).hexdigest()
    compose_path = OUT / "compose.yml"
    compose_path.write_text(yaml.safe_dump(compose, sort_keys=False), encoding="utf-8")
    cmd = ["docker", "compose", "-p", PROJECT, "-f", str(compose_path)]
    if candidate:
        verify_source(candidate, source, env, "before")
        source_identity = json.loads((candidate / "candidate.json").read_text(encoding="utf-8"))["source_id"]
    else:
        source_identity = "development:" + subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
    identity = {
        "project": PROJECT,
        "source": str(source),
        "dist": str(dist),
        "source_id": source_identity,
        "harness_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "mounted_assets_sha256": mounted_assets,
        "dist_sha256": {path.relative_to(dist).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(dist.rglob("*")) if path.is_file()},
        "targets": [BASE, "http://127.0.0.1:19383", "http://host.docker.internal:11434"],
        "images": {name: command(["docker", "image", "inspect", "--format", "{{.Id}}", svc["image"]], env=env, log=f"image-{name}.log").stdout.strip() for name, svc in compose["services"].items()},
    }
    with urlopen("http://127.0.0.1:11434/api/tags", timeout=30) as response:
        tags = {model["name"]: model for model in json.load(response)["models"]}
    identity["ollama_models"] = {name: {key: tags[name][key] for key in ("digest", "size")} for name, _ in MODELS}
    assert all(model["size"] > 1_000_000 for model in identity["ollama_models"].values()), "Only installed local model weights are allowed"
    if candidate:
        assert args.frontend_archive, "Frozen live proof requires the matching frontend artifact"
        archive = args.frontend_archive.resolve()
        command(
            [sys.executable, "-B", str(source / "tools/quality/frontend_artifact.py"), "verify", "--candidate", str(candidate), "--archive", str(archive)], env=env, log="frontend-verification.log"
        )
        receipt = json.loads(Path(str(archive) + ".json").read_text(encoding="utf-8"))
        assert identity["dist_sha256"] == {name: value["sha256"] for name, value in receipt["files"].items()}, "Mounted dist differs from the verified candidate build"
        identity["frontend_receipt_id"] = receipt["receipt_id"]
    (OUT / "identity.json").write_text(json.dumps(identity, indent=2), encoding="utf-8")
    exit_code = 1
    try:
        print("Starting disposable T1 stack", flush=True)
        command([*cmd, "up", "-d", "--pull", "never"], env=env, log="startup.log")
        deadline = time.monotonic() + 480
        while True:
            try:
                api("/api/v1/auth/login/channels")
                break
            except Exception:
                if time.monotonic() > deadline:
                    raise RuntimeError("Backend readiness timeout")
                time.sleep(3)
        print("Backend ready; initializing canonical empty QA model catalog", flush=True)
        check_assets = (
            "import json,hashlib;from pathlib import Path;expected="
            + repr(mounted_assets)
            + ";assert all(hashlib.sha256(Path(p).read_bytes()).hexdigest()==h for p,h in expected.items());print(json.dumps({'candidate_assets_verified':len(expected)}))"
        )
        command([*cmd, "exec", "-T", "app", "/ragflow/.venv/bin/python", "-"], env=env, log="runtime-assets.log", input=check_assets)
        seed = Path(__file__).with_name("seed_catalog.py").read_text(encoding="utf-8")
        command([*cmd, "exec", "-T", "app", "/ragflow/.venv/bin/python", "-"], env=env, log="catalog.log", input=seed)
        sys.path.insert(0, str(source))
        from test.playwright.conftest import _rsa_encrypt_password

        encrypted = _rsa_encrypt_password(env["QA_ADMIN_PASSWORD"])
        SECRET_VALUES.append(encrypted)
        user, token = api("/api/v1/auth/login", "POST", {"email": env["QA_ADMIN_EMAIL"], "password": encrypted})
        assert token and user.get("is_superuser"), "Synthetic managed-resource admin login failed"
        SECRET_VALUES.append(token)
        print("Synthetic superuser authenticated; configuring real local Ollama", flush=True)
        for model, kind in MODELS:
            api("/v1/llm/add_llm", "POST", {"llm_factory": "Ollama", "llm_name": model, "model_type": kind, "api_base": "http://host.docker.internal:11434", "max_tokens": 4096}, token)
            print(f"Validated local {kind} model {model}", flush=True)
        api("/api/v1/providers", "PUT", {"provider_name": "Ollama"}, token)
        api(
            "/api/v1/providers/Ollama/instances",
            "POST",
            {
                "instance_name": "QA",
                "api_key": "",
                "base_url": "http://host.docker.internal:11434",
                "region": "default",
                "model_info": [{"model_name": model, "model_type": [kind], "max_tokens": 4096} for model, kind in MODELS],
            },
            token,
        )
        for model, kind in (MODELS[0], MODELS[-1]):
            api("/api/v1/models/default", "PATCH", {"model_provider": "Ollama", "model_instance": "QA", "model_name": model, "model_type": kind}, token)
        tenant, _ = api("/api/v1/users/me/models", token=token)
        api(
            "/api/v1/users/me/models",
            "PATCH",
            {"tenant_id": tenant["tenant_id"], "llm_id": "qwen2.5:7b-instruct@QA@Ollama", "embd_id": "bge-m3:latest@QA@Ollama", "img2txt_id": "", "asr_id": "", "rerank_id": "", "tts_id": ""},
            token,
        )
        env.update(
            RAGFLOW_BASE_URL=BASE,
            SEEDED_USER_EMAIL=env["QA_ADMIN_EMAIL"],
            SEEDED_USER_PASSWORD=env["QA_ADMIN_PASSWORD"],
            PW_BROWSER="chromium",
            PW_TRACE="0",
            RUN_ID="t1-live-" + secrets.token_hex(4),
            PW_STEP_LOG="1",
            PYTHONPATH=str(source),
            QA_LIVE_TOKEN=token,
            QA_LIVE_EVIDENCE=str(OUT),
            PYTHONDONTWRITEBYTECODE="1",
            QA_DISPOSABLE_PROJECT=PROJECT,
            PW_ARTIFACTS_DIR=str(OUT / "artifacts"),
        )
        (OUT / "bootstrap.json").write_text(json.dumps({"admin_authenticated": True, "real_models_validated": True}), encoding="utf-8")
        print("Running real browser journeys", flush=True)
        tests = args.tests or [
            str(source / "test/playwright/e2e" / filename)
            for filename in ("test_live_document_roundtrip.py", "test_dataset_upload_parse.py", "test_next_apps_chat.py", "test_next_apps_search.py", "test_business_document_live_intake.py")
        ]
        # Registration through pytest_configure occurs after pytest-playwright,
        # so the explicit test profile wins without mutating production config.
        profile = """import pytest, os
class Profile:
 @pytest.fixture(scope="session")
 def browser_context_args(self):
  base=os.environ["RAGFLOW_BASE_URL"]
  return {"viewport":{"width":1920,"height":1080},"locale":"en-US","storage_state":{"cookies":[],"origins":[{"origin":base,"localStorage":[{"name":"lng","value":"en"}]}]}}
def pytest_configure(config):
 config.pluginmanager.register(Profile(),"t1-english")
"""
        (OUT / "live_profile.py").write_text(profile, encoding="utf-8")
        env["PYTHONPATH"] += os.pathsep + str(OUT)
        result = command(
            [
                sys.executable,
                "-B",
                "-m",
                "pytest",
                "-p",
                "live_profile",
                *tests,
                "--color=no",
                "--tb=short",
                "-rA",
                "-s",
                "-o",
                f"cache_dir={OUT / 'pytest-cache'}",
                f"--junitxml={OUT / 'browser.xml'}",
            ],
            env=env,
            log="browser.log",
            check=False,
            cwd=source,
            timeout=1800,
        )
        exit_code = result.returncode
        print(f"Live browser exit={exit_code}; see browser.log", flush=True)
    except Exception as exc:
        (OUT / "failure.txt").write_text(sanitized(str(exc)), encoding="utf-8")
        print(sanitized(str(exc)), flush=True)
    finally:
        try:
            command([*cmd, "logs", "--no-color", "--tail", "250", "app"], env=env, log="app.log", check=False)
        finally:
            command([*cmd, "down", "--volumes", "--remove-orphans"], env=env, log="cleanup.log", check=False)
        remain = command(["docker", "ps", "-a", "-q", "--filter", f"label=com.docker.compose.project={PROJECT}"], env=env, log="remaining-containers.log").stdout.strip()
        volumes = command(["docker", "volume", "ls", "-q", "--filter", f"label=com.docker.compose.project={PROJECT}"], env=env, log="remaining-volumes.log").stdout.strip()
        network = command(["docker", "network", "ls", "-q", "--filter", f"label=com.docker.compose.project={PROJECT}"], env=env, log="remaining-networks.log").stdout.strip()
        assert not remain and not volumes and not network, "Disposable resources remain; see cleanup logs"
        if candidate:
            verify_source(candidate, source, env, "after")
        print("Own containers, volumes and network removed", flush=True)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
