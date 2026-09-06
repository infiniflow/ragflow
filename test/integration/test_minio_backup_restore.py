"""Exercise the deployment's raw MinIO archive format on disposable Docker volumes.

Opt in with RAGFLOW_MINIO_BACKUP_TEST=1. No existing container or volume is used.
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
import pytest
import urllib3


def _docker(*arguments, env=None):
    result = subprocess.run(["docker", *arguments], env=env, capture_output=True, text=True, timeout=120, check=False)
    assert result.returncode == 0, result.stderr[-3000:]
    return result.stdout.strip()


@pytest.mark.p0
def test_minio_raw_archive_restores_object_bytes_and_metadata(tmp_path):
    if os.environ.get("RAGFLOW_MINIO_BACKUP_TEST") != "1":
        pytest.skip("Requires explicitly enabled disposable Docker MinIO backup test")
    token = uuid4().hex[:16]
    volumes = [f"t1-minio-{token}-{suffix}" for suffix in ("source", "restore")]
    containers = [f"t1-minio-{token}-{suffix}" for suffix in ("source", "restore")]
    minio_image = os.environ.get("RAGFLOW_MINIO_TEST_IMAGE", "pgsty/minio:RELEASE.2026-03-25T00-00-00Z")
    backup_image = os.environ.get("RAGFLOW_BACKUP_TEST_IMAGE", "alpine:3.22")
    # Never pull implicitly or print credentials. Each instance owns an empty volume.
    _docker("image", "inspect", minio_image, "--format", "{{.Id}}")
    _docker("image", "inspect", backup_image, "--format", "{{.Id}}")
    environment = {**os.environ, "MINIO_ROOT_USER": "t1-synthetic", "MINIO_ROOT_PASSWORD": secrets.token_urlsafe(32)}
    created_volumes = []
    created_containers = []
    backup = Path(tmp_path).resolve()
    bucket = "t1-documents"
    expected = {
        "tenant-a/evidence/source.txt": (b"Synthetic evidence: cobalt lighthouse 42\n", "evidence", "tenant-a"),
        "tenant-a/exports/report.docx": (b"Synthetic export bytes\x00\xff\n", "export", "tenant-a"),
        "tenant-b/evidence/empty.txt": (b"", "evidence", "tenant-b"),
    }

    def start(index):
        name = containers[index]
        _docker(
            "run",
            "-d",
            "--pull=never",
            "--name",
            name,
            "--label",
            "codex.task=t1-minio-backup",
            "-p",
            "127.0.0.1::9000",
            "-v",
            f"{volumes[index]}:/data",
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
        return connect(index)

    def connect(index):
        endpoint = _docker("port", containers[index], "9000/tcp")
        assert endpoint.startswith("127.0.0.1:")
        client = Minio(
            endpoint,
            access_key=environment["MINIO_ROOT_USER"],
            secret_key=environment["MINIO_ROOT_PASSWORD"],
            secure=False,
            http_client=urllib3.PoolManager(timeout=urllib3.Timeout(connect=2, read=5), retries=False),
        )
        deadline = time.monotonic() + 40
        while True:
            try:
                client.list_buckets()
                return client
            except Exception:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.2)

    def verify(client):
        assert [item.name for item in client.list_buckets()] == [bucket]
        assert {item.object_name for item in client.list_objects(bucket, recursive=True)} == set(expected)
        for key, (payload, kind, tenant) in expected.items():
            response = client.get_object(bucket, key)
            try:
                assert response.read() == payload
            finally:
                response.close()
                response.release_conn()
            stat = client.stat_object(bucket, key)
            assert stat.size == len(payload)
            assert stat.content_type == "application/octet-stream"
            metadata = {key.lower(): value for key, value in stat.metadata.items()}
            assert metadata["x-amz-meta-sha256"] == hashlib.sha256(payload).hexdigest()
            assert metadata["x-amz-meta-kind"] == kind
            assert metadata["x-amz-meta-tenant"] == tenant

    try:
        for volume in volumes:
            _docker("volume", "create", "--label", "codex.task=t1-minio-backup", volume)
            created_volumes.append(volume)
        source = start(0)
        source.make_bucket(bucket)
        for key, (payload, kind, tenant) in expected.items():
            source.put_object(
                bucket,
                key,
                io.BytesIO(payload),
                len(payload),
                content_type="application/octet-stream",
                metadata={
                    "sha256": hashlib.sha256(payload).hexdigest(),
                    "kind": kind,
                    "tenant": tenant,
                },
            )
        verify(source)
        # Deployment archives /data with tar. Stop writes first for a consistent snapshot.
        _docker("stop", containers[0])
        _docker(
            "run",
            "--rm",
            "--pull=never",
            "--network=none",
            "--volumes-from",
            f"{containers[0]}:ro",
            "-v",
            f"{backup}:/backup",
            "--entrypoint",
            "/bin/tar",
            backup_image,
            "-czf",
            "/backup/minio-data.tar.gz",
            "-C",
            "/data",
            ".",
        )
        listing = _docker("run", "--rm", "--pull=never", "--network=none", "-v", f"{backup}:/backup:ro", "--entrypoint", "/bin/tar", backup_image, "-tzf", "/backup/minio-data.tar.gz")
        assert "t1-documents" in listing
        _docker(
            "run",
            "--rm",
            "--pull=never",
            "--network=none",
            "-v",
            f"{backup}:/backup:ro",
            "-v",
            f"{volumes[1]}:/data",
            "--entrypoint",
            "/bin/tar",
            backup_image,
            "-xzf",
            "/backup/minio-data.tar.gz",
            "-C",
            "/data",
        )
        restored = start(1)
        verify(restored)
        # Removal from the restored copy proves independent storage after recovery.
        restored.remove_object(bucket, "tenant-a/exports/report.docx")
        _docker("start", containers[0])
        verify(connect(0))
    finally:
        for name in reversed(created_containers):
            _docker("rm", "-f", name)
        for name in reversed(created_volumes):
            _docker("volume", "rm", name)
        assert not _docker("ps", "-aq", "--filter", f"name=t1-minio-{token}")
        assert not _docker("volume", "ls", "-q", "--filter", f"name=t1-minio-{token}")
