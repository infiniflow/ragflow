"""Coordinated PostgreSQL and MinIO restore on disposable Docker resources.

Opt in with ``RAGFLOW_COORDINATED_BACKUP_TEST=1``.  The test creates unique
containers and volumes, writes only synthetic data, freezes its only writers,
and verifies that one snapshot identifier links the restored database row to
the restored object and metadata.
"""

import hashlib
import io
import os
from pathlib import Path
import secrets
import subprocess
import time
from uuid import uuid4

from minio import Minio
import psycopg2
import pytest
import urllib3


def _run(*arguments: str, env=None, input_data: bytes | None = None, binary: bool = False):
    result = subprocess.run(
        list(arguments),
        env=env,
        input=input_data,
        capture_output=True,
        text=not binary,
        timeout=180,
        check=False,
    )
    stderr = result.stderr if isinstance(result.stderr, str) else result.stderr.decode("utf-8", "replace")
    assert result.returncode == 0, stderr[-3000:]
    return result.stdout


def _docker(*arguments: str, **kwargs):
    return _run("docker", *arguments, **kwargs)


@pytest.mark.p0
def test_coordinated_postgres_and_minio_snapshot_restores_linked_data(tmp_path: Path):
    if os.environ.get("RAGFLOW_COORDINATED_BACKUP_TEST") != "1":
        pytest.skip("Requires explicitly enabled disposable PostgreSQL and MinIO Docker test")

    token = uuid4().hex[:16]
    postgres_image = os.environ.get("RAGFLOW_POSTGRES_TEST_IMAGE", "postgres:16-alpine")
    minio_image = os.environ.get("RAGFLOW_MINIO_TEST_IMAGE", "pgsty/minio:RELEASE.2026-03-25T00-00-00Z")
    backup_image = os.environ.get("RAGFLOW_BACKUP_TEST_IMAGE", "alpine:3.22")
    for image in (postgres_image, minio_image, backup_image):
        _docker("image", "inspect", image, "--format", "{{.Id}}")

    names = {
        "pg_source": f"t1-coordinated-{token}-pg-source",
        "pg_restore": f"t1-coordinated-{token}-pg-restore",
        "minio_source": f"t1-coordinated-{token}-minio-source",
        "minio_restore": f"t1-coordinated-{token}-minio-restore",
    }
    volumes = {key: value + "-data" for key, value in names.items()}
    created_containers: list[str] = []
    created_volumes: list[str] = []
    password = secrets.token_urlsafe(32)
    minio_secret = secrets.token_urlsafe(32)
    environment = {
        **os.environ,
        "POSTGRES_PASSWORD": password,
        "PGPASSWORD": password,
        "MINIO_ROOT_USER": "t1-synthetic",
        "MINIO_ROOT_PASSWORD": minio_secret,
    }
    snapshot_id = "snapshot-" + uuid4().hex
    bucket = "t1-coordinated"
    object_key = "tenant-fixture/document.bin"
    payload = b"Synthetic coordinated PostgreSQL and object-store evidence\x00\xff\n"
    digest = hashlib.sha256(payload).hexdigest()
    backup_dir = Path(tmp_path).resolve()

    def create_volume(name: str) -> None:
        _docker("volume", "create", "--label", "codex.task=t1-coordinated-backup", name)
        created_volumes.append(name)

    def host_port(container: str, port: str) -> int:
        endpoint = _docker("port", container, port).strip()
        assert endpoint.startswith("127.0.0.1:")
        return int(endpoint.rsplit(":", 1)[1])

    def start_postgres(key: str) -> int:
        name = names[key]
        _docker(
            "run",
            "-d",
            "--pull=never",
            "--name",
            name,
            "--label",
            "codex.task=t1-coordinated-backup",
            "-p",
            "127.0.0.1::5432",
            "-v",
            f"{volumes[key]}:/var/lib/postgresql/data",
            "--env",
            "POSTGRES_PASSWORD",
            "-e",
            "POSTGRES_USER=t1",
            "-e",
            "POSTGRES_DB=t1",
            postgres_image,
            env=environment,
        )
        created_containers.append(name)
        port = host_port(name, "5432/tcp")
        deadline = time.monotonic() + 60
        while True:
            try:
                connection = psycopg2.connect(host="127.0.0.1", port=port, user="t1", password=password, dbname="t1", connect_timeout=2)
                connection.close()
                return port
            except psycopg2.Error:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.25)

    def connect_minio(name: str) -> tuple[Minio, int]:
        port = host_port(name, "9000/tcp")
        client = Minio(
            f"127.0.0.1:{port}",
            access_key=environment["MINIO_ROOT_USER"],
            secret_key=environment["MINIO_ROOT_PASSWORD"],
            secure=False,
            http_client=urllib3.PoolManager(timeout=urllib3.Timeout(connect=2, read=5), retries=False),
        )
        deadline = time.monotonic() + 60
        while True:
            try:
                client.list_buckets()
                return client, port
            except Exception:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.25)

    def start_minio(key: str) -> tuple[Minio, int]:
        name = names[key]
        _docker(
            "run",
            "-d",
            "--pull=never",
            "--name",
            name,
            "--label",
            "codex.task=t1-coordinated-backup",
            "-p",
            "127.0.0.1::9000",
            "-v",
            f"{volumes[key]}:/data",
            "--env",
            "MINIO_ROOT_USER",
            "--env",
            "MINIO_ROOT_PASSWORD",
            minio_image,
            "server",
            "/data",
            env=environment,
        )
        created_containers.append(name)
        return connect_minio(name)

    def verify_linkage(postgres_port: int, minio: Minio) -> None:
        with psycopg2.connect(host="127.0.0.1", port=postgres_port, user="t1", password=password, dbname="t1") as connection:
            with connection.cursor() as cursor:
                cursor.execute("SELECT snapshot_id, bucket, object_key, sha256, byte_length FROM fixture_manifest")
                assert cursor.fetchall() == [(snapshot_id, bucket, object_key, digest, len(payload))]
        response = minio.get_object(bucket, object_key)
        try:
            assert response.read() == payload
        finally:
            response.close()
            response.release_conn()
        metadata = {key.lower(): value for key, value in minio.stat_object(bucket, object_key).metadata.items()}
        assert metadata["x-amz-meta-snapshot-id"] == snapshot_id
        assert metadata["x-amz-meta-sha256"] == digest

    try:
        for volume in volumes.values():
            create_volume(volume)
        source_pg_port = start_postgres("pg_source")
        source_minio, _ = start_minio("minio_source")
        source_minio.make_bucket(bucket)
        source_minio.put_object(
            bucket,
            object_key,
            io.BytesIO(payload),
            len(payload),
            content_type="application/octet-stream",
            metadata={"snapshot-id": snapshot_id, "sha256": digest},
        )
        with psycopg2.connect(host="127.0.0.1", port=source_pg_port, user="t1", password=password, dbname="t1") as connection:
            with connection.cursor() as cursor:
                cursor.execute("CREATE TABLE fixture_manifest (snapshot_id text PRIMARY KEY, bucket text NOT NULL, object_key text NOT NULL, sha256 text NOT NULL, byte_length bigint NOT NULL)")
                cursor.execute("INSERT INTO fixture_manifest VALUES (%s, %s, %s, %s, %s)", (snapshot_id, bucket, object_key, digest, len(payload)))
        verify_linkage(source_pg_port, source_minio)

        # This test owns every writer.  After the manifest commit and client
        # closure, no writes can occur while the two snapshot artifacts are made.
        _docker("stop", names["minio_source"])
        database_dump = _docker(
            "exec",
            "-i",
            "-e",
            "PGPASSWORD",
            names["pg_source"],
            "pg_dump",
            "-U",
            "t1",
            "-d",
            "t1",
            "-Fc",
            "--no-owner",
            "--no-acl",
            env=environment,
            binary=True,
        )
        (backup_dir / "postgres.dump").write_bytes(database_dump)
        _docker(
            "run",
            "--rm",
            "--pull=never",
            "--network=none",
            "--volumes-from",
            f"{names['minio_source']}:ro",
            "-v",
            f"{backup_dir}:/backup",
            "--entrypoint",
            "/bin/tar",
            backup_image,
            "-czf",
            "/backup/minio-data.tar.gz",
            "-C",
            "/data",
            ".",
        )

        _docker(
            "run",
            "--rm",
            "--pull=never",
            "--network=none",
            "-v",
            f"{backup_dir}:/backup:ro",
            "-v",
            f"{volumes['minio_restore']}:/data",
            "--entrypoint",
            "/bin/tar",
            backup_image,
            "-xzf",
            "/backup/minio-data.tar.gz",
            "-C",
            "/data",
        )
        restored_pg_port = start_postgres("pg_restore")
        _docker(
            "exec",
            "-i",
            "-e",
            "PGPASSWORD",
            names["pg_restore"],
            "pg_restore",
            "-U",
            "t1",
            "-d",
            "t1",
            "--exit-on-error",
            "--no-owner",
            "--no-acl",
            env=environment,
            input_data=database_dump,
            binary=True,
        )
        restored_minio, _ = start_minio("minio_restore")
        verify_linkage(restored_pg_port, restored_minio)

        # Negative control: a partial restore must be detected by the same
        # linkage check rather than reported as a successful database restore.
        restored_minio.remove_object(bucket, object_key)
        with pytest.raises(Exception):
            verify_linkage(restored_pg_port, restored_minio)
        _docker("start", names["minio_source"])
        verify_linkage(source_pg_port, connect_minio(names["minio_source"])[0])
    finally:
        for name in reversed(created_containers):
            _docker("rm", "-f", name)
        for name in reversed(created_volumes):
            _docker("volume", "rm", name)
        assert not _docker("ps", "-aq", "--filter", f"name=t1-coordinated-{token}").strip()
        assert not _docker("volume", "ls", "-q", "--filter", f"name=t1-coordinated-{token}").strip()
