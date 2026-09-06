from fastapi.testclient import TestClient

from asr_service.main import app


def test_ready_not_ready_when_ffmpeg_missing(monkeypatch):
    checks = {
        "ffmpeg": {"ok": False, "details": {"resolved_path": None, "checked_at": "now", "error": "tool not found"}},
        "sox": {"ok": True, "details": {"resolved_path": None, "checked_at": "now", "error": "check skipped (normalization disabled)"}},
    }
    monkeypatch.setattr("asr_service.settings.settings.enable_sox_normalize", False)
    monkeypatch.setattr("asr_service.main._build_health_checks", lambda: checks)

    with TestClient(app) as client:
        response = client.get("/health/ready")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "not_ready"
    assert body["checks"]["ffmpeg"]["ok"] is False


def test_ready_not_ready_when_sox_missing_with_normalize_enabled(monkeypatch):
    checks = {
        "ffmpeg": {"ok": True, "details": {"resolved_path": "ffmpeg", "checked_at": "now", "error": None}},
        "sox": {"ok": False, "details": {"resolved_path": None, "checked_at": "now", "error": "tool not found"}},
    }
    monkeypatch.setattr("asr_service.settings.settings.enable_sox_normalize", True)
    monkeypatch.setattr("asr_service.main._build_health_checks", lambda: checks)

    with TestClient(app) as client:
        response = client.get("/health/ready")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "not_ready"
    assert body["checks"]["sox"]["ok"] is False


def test_ready_when_required_checks_ok(monkeypatch):
    checks = {
        "ffmpeg": {"ok": True, "details": {"resolved_path": "ffmpeg", "checked_at": "now", "error": None}},
        "sox": {"ok": True, "details": {"resolved_path": "sox", "checked_at": "now", "error": None}},
    }
    monkeypatch.setattr("asr_service.settings.settings.enable_sox_normalize", True)
    monkeypatch.setattr("asr_service.main._build_health_checks", lambda: checks)

    with TestClient(app) as client:
        response = client.get("/health/ready")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "ready"
    assert set(body["checks"].keys()) == {"ffmpeg", "sox"}
