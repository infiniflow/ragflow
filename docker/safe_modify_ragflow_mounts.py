#!/usr/bin/env python3

from __future__ import annotations

import difflib
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from pathlib import Path


WORKDIR = Path("/home/ljq/ragflow/docker")
BASE_FILE = WORKDIR / "docker-compose-base.yml"
COMPOSE_FILE = WORKDIR / "docker-compose.yml"
STAMP_FILE = Path.home() / ".ragflow_compose_backup_stamp"

TARGET_ROOT = Path("/7.48T_1/ragflow-data")

TARGET_DIRS = {
    "elasticsearch": TARGET_ROOT / "elasticsearch",
    "mysql": TARGET_ROOT / "mysql",
    "minio": TARGET_ROOT / "minio",
    "redis": TARGET_ROOT / "redis",
    "logs": TARGET_ROOT / "logs",
}

RAGFLOW_CONTAINERS = {
    "docker-ragflow-cpu-1",
    "docker-ragflow-gpu-1",
    "docker-es01-1",
    "docker-minio-1",
    "docker-redis-1",
    "docker-mysql-1",
}


class MigrationError(RuntimeError):
    pass


def run(
    args: list[str],
    *,
    check: bool = True,
    capture: bool = True,
) -> subprocess.CompletedProcess[str]:
    """安全运行系统命令。"""
    return subprocess.run(
        args,
        check=check,
        text=True,
        capture_output=capture,
    )


def atomic_write(path: Path, content: str) -> None:
    """在同一目录内原子替换文件，并保留原文件权限。"""
    old_stat = path.stat()

    fd, temp_name = tempfile.mkstemp(
        prefix=f".{path.name}.",
        suffix=".tmp",
        dir=path.parent,
        text=True,
    )

    temp_path = Path(temp_name)

    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as file:
            file.write(content)
            file.flush()
            os.fsync(file.fileno())

        os.chmod(temp_path, stat.S_IMODE(old_stat.st_mode))
        os.replace(temp_path, path)

    finally:
        if temp_path.exists():
            temp_path.unlink()


def replace_pattern(
    text: str,
    *,
    pattern: str,
    replacement: str,
    expected_count: int,
    description: str,
) -> str:
    """仅在匹配数量完全符合预期时替换。"""
    result, actual_count = re.subn(
        pattern,
        replacement,
        text,
        flags=re.MULTILINE,
    )

    if actual_count != expected_count:
        raise MigrationError(
            f"{description}：预期匹配 {expected_count} 处，"
            f"实际匹配 {actual_count} 处，停止修改。"
        )

    print(
        f"[通过] {description}："
        f"找到并替换 {actual_count} 处"
    )
    return result


def check_backup() -> tuple[Path, Path]:
    """确认用户刚才创建的备份存在。"""
    if not STAMP_FILE.is_file():
        raise MigrationError(
            f"未找到备份时间记录：{STAMP_FILE}"
        )

    stamp = STAMP_FILE.read_text(
        encoding="utf-8"
    ).strip()

    if not stamp:
        raise MigrationError("备份时间记录为空。")

    backup_base = WORKDIR / (
        f"docker-compose-base.yml.bak.{stamp}"
    )
    backup_compose = WORKDIR / (
        f"docker-compose.yml.bak.{stamp}"
    )

    for path in (backup_base, backup_compose):
        if not path.is_file():
            raise MigrationError(
                f"未找到备份文件：{path}"
            )

    print(f"[通过] 备份时间：{stamp}")
    print(f"[通过] 基础配置备份：{backup_base.name}")
    print(f"[通过] 主配置备份：{backup_compose.name}")

    return backup_base, backup_compose


def check_files_unchanged(
    backup_base: Path,
    backup_compose: Path,
) -> None:
    """确认备份后配置文件未被手工修改。"""
    pairs = [
        (BASE_FILE, backup_base),
        (COMPOSE_FILE, backup_compose),
    ]

    for current, backup in pairs:
        current_data = current.read_bytes()
        backup_data = backup.read_bytes()

        if current_data != backup_data:
            raise MigrationError(
                f"{current.name} 与刚才的备份不一致。\n"
                "说明备份后文件发生过修改，脚本为安全起见停止。"
            )

    print("[通过] 当前配置与刚才的备份完全一致")


def check_containers_stopped() -> None:
    """禁止在 RAGFlow 仍运行时修改挂载配置。"""
    result = run(
        ["docker", "ps", "--format", "{{.Names}}"]
    )

    running = {
        line.strip()
        for line in result.stdout.splitlines()
        if line.strip()
    }

    active = sorted(running & RAGFLOW_CONTAINERS)

    if active:
        raise MigrationError(
            "以下 RAGFlow 容器仍在运行：\n  "
            + "\n  ".join(active)
            + "\n请先执行 docker compose down，且不要添加 -v。"
        )

    print("[通过] RAGFlow 相关容器均已停止")


def get_mount_source(path: str) -> str:
    result = run(
        [
            "findmnt",
            "-n",
            "-o",
            "SOURCE",
            "-T",
            path,
        ]
    )
    return result.stdout.strip()


def check_target_disk() -> None:
    """确认目标目录不位于系统根分区。"""
    root_device = get_mount_source("/")
    target_device = get_mount_source("/7.48T_1")

    if not root_device or not target_device:
        raise MigrationError(
            "无法识别系统盘或目标盘挂载设备。"
        )

    print(f"[信息] 系统盘设备：{root_device}")
    print(f"[信息] 目标盘设备：{target_device}")

    if root_device == target_device:
        raise MigrationError(
            "/7.48T_1 与系统根目录位于同一个设备，停止修改。"
        )

    print("[通过] /7.48T_1 位于其他磁盘设备")


def check_target_directories() -> None:
    """确认所有目标目录存在，核心数据目录不为空。"""
    for name, path in TARGET_DIRS.items():
        if not path.is_dir():
            raise MigrationError(
                f"目标目录不存在：{path}"
            )

        print(f"[通过] 目标目录存在：{path}")

    core_directories = {
        "Elasticsearch": TARGET_DIRS["elasticsearch"],
        "MySQL": TARGET_DIRS["mysql"],
        "MinIO": TARGET_DIRS["minio"],
    }

    for name, path in core_directories.items():
        try:
            has_content = next(path.iterdir(), None) is not None
        except PermissionError as exc:
            raise MigrationError(
                f"没有权限读取 {path}，请使用 sudo 检查数据。"
            ) from exc

        if not has_content:
            raise MigrationError(
                f"{name} 目标目录为空：{path}\n"
                "核心数据可能尚未复制，停止修改。"
            )

        print(f"[通过] {name} 目标目录不是空目录")


def build_modified_files(
    base_text: str,
    compose_text: str,
) -> tuple[str, str]:
    """基于已确认的原始挂载执行精确替换。"""

    base_text = replace_pattern(
        base_text,
        pattern=(
            r"^ {6}- esdata01:"
            r"/usr/share/elasticsearch/data[ \t]*$"
        ),
        replacement=(
            "      - type: bind\n"
            "        source: /7.48T_1/ragflow-data/elasticsearch\n"
            "        target: /usr/share/elasticsearch/data\n"
            "        bind:\n"
            "          create_host_path: false"
        ),
        expected_count=1,
        description="Elasticsearch 数据挂载",
    )

    base_text = replace_pattern(
        base_text,
        pattern=(
            r"^ {6}- mysql_data:"
            r"/var/lib/mysql[ \t]*$"
        ),
        replacement=(
            "      - type: bind\n"
            "        source: /7.48T_1/ragflow-data/mysql\n"
            "        target: /var/lib/mysql\n"
            "        bind:\n"
            "          create_host_path: false"
        ),
        expected_count=1,
        description="MySQL 数据挂载",
    )

    base_text = replace_pattern(
        base_text,
        pattern=(
            r"^ {6}- minio_data:/data[ \t]*$"
        ),
        replacement=(
            "      - type: bind\n"
            "        source: /7.48T_1/ragflow-data/minio\n"
            "        target: /data\n"
            "        bind:\n"
            "          create_host_path: false"
        ),
        expected_count=1,
        description="MinIO 数据挂载",
    )

    base_text = replace_pattern(
        base_text,
        pattern=(
            r"^ {6}- redis_data:/data[ \t]*$"
        ),
        replacement=(
            "      - type: bind\n"
            "        source: /7.48T_1/ragflow-data/redis\n"
            "        target: /data\n"
            "        bind:\n"
            "          create_host_path: false"
        ),
        expected_count=1,
        description="Redis 数据挂载",
    )

    compose_text = replace_pattern(
        compose_text,
        pattern=(
            r"^ {6}- \./ragflow-logs:"
            r"/ragflow/logs[ \t]*$"
        ),
        replacement=(
            "      - type: bind\n"
            "        source: /7.48T_1/ragflow-data/logs\n"
            "        target: /ragflow/logs\n"
            "        bind:\n"
            "          create_host_path: false"
        ),
        expected_count=2,
        description="CPU/GPU RAGFlow 日志挂载",
    )

    # 确认注释中的日志挂载仍然存在且未被修改。
    comment_pattern = re.compile(
        r"^[ \t]*#[ \t]*-[ \t]+"
        r"\./ragflow-logs:/ragflow/logs[ \t]*$",
        flags=re.MULTILINE,
    )

    comment_count = len(
        comment_pattern.findall(compose_text)
    )

    if comment_count != 1:
        raise MigrationError(
            "预期保留 1 处被注释的 ragflow-logs 挂载，"
            f"实际发现 {comment_count} 处。"
        )

    print("[通过] 第117行附近的注释挂载保持不变")

    return base_text, compose_text


def validate_modified_text(
    base_text: str,
    compose_text: str,
) -> None:
    """对修改结果做静态数量检查。"""
    if base_text.count("create_host_path: false") != 4:
        raise MigrationError(
            "docker-compose-base.yml 中 "
            "create_host_path 数量不是 4。"
        )

    if compose_text.count("create_host_path: false") != 2:
        raise MigrationError(
            "docker-compose.yml 中 "
            "create_host_path 数量不是 2。"
        )

    old_active_patterns = [
        r"^[ \t]*-[ \t]+esdata01:"
        r"/usr/share/elasticsearch/data[ \t]*$",
        r"^[ \t]*-[ \t]+mysql_data:"
        r"/var/lib/mysql[ \t]*$",
        r"^[ \t]*-[ \t]+minio_data:/data[ \t]*$",
        r"^[ \t]*-[ \t]+redis_data:/data[ \t]*$",
        r"^ {6}- \./ragflow-logs:/ragflow/logs[ \t]*$",
    ]

    combined = base_text + "\n" + compose_text

    for pattern in old_active_patterns:
        if re.search(pattern, combined, flags=re.MULTILINE):
            raise MigrationError(
                f"修改后仍发现旧的有效挂载：{pattern}"
            )

    print("[通过] 静态挂载数量检查通过")


def validate_with_docker_compose() -> str:
    """调用 Docker Compose 解析最终生效配置。"""
    command = [
        "docker",
        "compose",
        "-p",
        "docker",
        "-f",
        str(COMPOSE_FILE),
        "--profile",
        "cpu",
        "--profile",
        "elasticsearch",
        "config",
    ]

    result = run(command)

    rendered = result.stdout

    expected_sources = [
        "/7.48T_1/ragflow-data/elasticsearch",
        "/7.48T_1/ragflow-data/mysql",
        "/7.48T_1/ragflow-data/minio",
        "/7.48T_1/ragflow-data/redis",
        "/7.48T_1/ragflow-data/logs",
    ]

    missing = [
        source
        for source in expected_sources
        if source not in rendered
    ]

    if missing:
        raise MigrationError(
            "Compose 虽然能够解析，但最终配置缺少以下路径：\n  "
            + "\n  ".join(missing)
        )

    print("[通过] docker compose config 解析成功")
    print("[通过] 最终配置包含全部新硬盘路径")

    return rendered


def print_diff(
    backup: Path,
    current: Path,
) -> None:
    old_lines = backup.read_text(
        encoding="utf-8"
    ).splitlines(keepends=True)

    new_lines = current.read_text(
        encoding="utf-8"
    ).splitlines(keepends=True)

    diff = difflib.unified_diff(
        old_lines,
        new_lines,
        fromfile=backup.name,
        tofile=current.name,
    )

    print("".join(diff), end="")


def main() -> int:
    os.chdir(WORKDIR)

    backup_base, backup_compose = check_backup()
    check_files_unchanged(backup_base, backup_compose)
    check_containers_stopped()
    check_target_disk()
    check_target_directories()

    original_base = BASE_FILE.read_text(encoding="utf-8")
    original_compose = COMPOSE_FILE.read_text(encoding="utf-8")

    modified_base, modified_compose = build_modified_files(
        original_base,
        original_compose,
    )

    validate_modified_text(
        modified_base,
        modified_compose,
    )

    # 再创建一份脚本执行瞬间的保险备份。
    transaction_base = WORKDIR / (
        "docker-compose-base.yml.pre-transaction"
    )
    transaction_compose = WORKDIR / (
        "docker-compose.yml.pre-transaction"
    )

    shutil.copy2(BASE_FILE, transaction_base)
    shutil.copy2(COMPOSE_FILE, transaction_compose)

    print("[通过] 已创建事务前保险备份")

    try:
        atomic_write(BASE_FILE, modified_base)
        atomic_write(COMPOSE_FILE, modified_compose)

        rendered = validate_with_docker_compose()

        rendered_path = Path(
            "/tmp/ragflow-compose-migrated.yml"
        )
        rendered_path.write_text(
            rendered,
            encoding="utf-8",
        )

    except Exception:
        print(
            "\n[回滚] 修改或验证失败，正在恢复原配置……",
            file=sys.stderr,
        )

        atomic_write(BASE_FILE, original_base)
        atomic_write(COMPOSE_FILE, original_compose)

        print(
            "[回滚完成] 两个 Compose 文件已恢复。",
            file=sys.stderr,
        )
        raise

    print("\n========== docker-compose-base.yml 差异 ==========")
    print_diff(backup_base, BASE_FILE)

    print("\n========== docker-compose.yml 差异 ==========")
    print_diff(backup_compose, COMPOSE_FILE)

    print("\n修改成功，但尚未启动任何容器。")
    print(
        "最终渲染配置已保存到："
        "/tmp/ragflow-compose-migrated.yml"
    )

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except MigrationError as exc:
        print(f"\n[停止] {exc}", file=sys.stderr)
        raise SystemExit(1)
    except subprocess.CalledProcessError as exc:
        print(
            "\n[停止] 外部命令执行失败：",
            " ".join(exc.cmd),
            file=sys.stderr,
        )

        if exc.stdout:
            print(exc.stdout, file=sys.stderr)

        if exc.stderr:
            print(exc.stderr, file=sys.stderr)

        raise SystemExit(exc.returncode or 1)
