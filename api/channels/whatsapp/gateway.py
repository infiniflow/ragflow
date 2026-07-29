from __future__ import annotations

import asyncio
import logging
import os
import shlex
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

LOGGER = logging.getLogger(__name__)
_missing_command_warned = False
_deps_install_warned = False


def _env_flag(name: str, default: bool = False) -> bool:
    raw = os.getenv(name)
    if raw is None:
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def _default_gateway_command() -> list[str]:
    raw = os.getenv("WHATSAPP_GATEWAY_COMMAND", "").strip()
    if raw:
        return shlex.split(raw)
    gateway_entry = Path(__file__).resolve().parent / "gateway-node" / "index.js"
    node = shutil.which("node")
    if node and gateway_entry.exists():
        return [node, str(gateway_entry)]
    return []


def _gateway_dir() -> Path:
    return Path(__file__).resolve().parent / "gateway-node"


@dataclass
class WhatsAppGatewayConfig:
    command: list[str]
    cwd: str
    enabled: bool


class WhatsAppGatewayRuntime:
    def __init__(self) -> None:
        self._process: Optional[asyncio.subprocess.Process] = None
        self._lock = asyncio.Lock()
        self._install_lock = asyncio.Lock()
        self._sync_generation = 0

    def _config(self) -> WhatsAppGatewayConfig:
        workdir = os.getenv("WHATSAPP_GATEWAY_WORKDIR", "").strip()
        return WhatsAppGatewayConfig(
            command=_default_gateway_command(),
            cwd=workdir or str(_gateway_dir()),
            enabled=_env_flag("WHATSAPP_GATEWAY_ENABLED", True),
        )

    def is_running(self) -> bool:
        return bool(self._process and self._process.returncode is None)

    async def sync(self, enabled: bool) -> None:
        cfg = self._config()
        should_run = bool(enabled and cfg.enabled and cfg.command)
        async with self._lock:
            self._sync_generation += 1
            generation = self._sync_generation
            if not should_run:
                await self._stop_locked()
                return
            if self.is_running():
                return

        await self._ensure_dependencies(cfg)

        async with self._lock:
            if generation != self._sync_generation:
                return
            if not should_run:
                await self._stop_locked()
                return
            await self._start_locked(cfg)

    async def _ensure_dependencies(self, cfg: WhatsAppGatewayConfig) -> None:
        global _deps_install_warned
        if not _env_flag("WHATSAPP_GATEWAY_AUTO_INSTALL", True):
            return

        async with self._install_lock:
            gateway_dir = Path(cfg.cwd)
            node_modules = gateway_dir / "node_modules"
            if node_modules.exists():
                return

            npm = shutil.which("npm")
            if not npm:
                if not _deps_install_warned:
                    LOGGER.warning("npm is not available; WhatsApp gateway dependencies cannot be installed automatically")
                    _deps_install_warned = True
                return

            package_json = gateway_dir / "package.json"
            if not package_json.exists():
                LOGGER.warning("WhatsApp gateway package.json not found in %s", gateway_dir)
                return

            LOGGER.info("installing WhatsApp gateway dependencies in %s", gateway_dir)
            proc = await asyncio.create_subprocess_exec(
                npm,
                "install",
                "--no-fund",
                "--no-audit",
                cwd=str(gateway_dir),
            )
            try:
                code = await asyncio.wait_for(proc.wait(), timeout=300)
            except asyncio.TimeoutError as ex:
                proc.kill()
                await proc.wait()
                raise RuntimeError("npm install timed out after 300s") from ex
            if code != 0:
                raise RuntimeError(f"npm install failed with exit code {code}")
            _deps_install_warned = False

    async def _start_locked(self, cfg: WhatsAppGatewayConfig) -> None:
        global _missing_command_warned
        if self.is_running():
            return
        if not cfg.command:
            if not _missing_command_warned:
                LOGGER.warning("WhatsApp gateway command is not configured; gateway will not start")
                _missing_command_warned = True
            return
        _missing_command_warned = False

        env = os.environ.copy()
        env.setdefault("PYTHONUNBUFFERED", "1")
        LOGGER.info("starting WhatsApp gateway: %s", " ".join(cfg.command))
        self._process = await asyncio.create_subprocess_exec(
            *cfg.command,
            cwd=cfg.cwd,
            env=env,
        )

    async def _stop_locked(self) -> None:
        proc = self._process
        if proc is None:
            return

        if proc.returncode is None:
            LOGGER.info("stopping WhatsApp gateway")
            proc.terminate()
            try:
                await asyncio.wait_for(proc.wait(), timeout=10)
            except asyncio.TimeoutError:
                LOGGER.warning("WhatsApp gateway did not stop in time; killing it")
                proc.kill()
                await proc.wait()
            except Exception:
                LOGGER.debug("WhatsApp gateway stop failed", exc_info=True)

        self._process = None


_gateway_runtime = WhatsAppGatewayRuntime()


async def sync_whatsapp_gateway(enabled: bool) -> None:
    await _gateway_runtime.sync(enabled)
