"""Opt-in real T-one API contract using a known synthetic Russian recording.

Run as unittest in an isolated service environment with ASR_REAL_TONE_AUDIO set
to a WAV speaking EXPECTED_TRANSCRIPT. No engine, preprocessing or HTTP mocks.
The normal unit suite skips this lane when the real audio fixture is absent.
"""

import json
import os
from pathlib import Path
import re
import time
import unittest


EXPECTED_TRANSCRIPT = "привет это проверка распознавания речи сегодня хорошая погода"


@unittest.skipUnless(os.environ.get("ASR_REAL_TONE_AUDIO"), "real T-one lane requires synthetic audio and installed model/runtime")
class RealToneInferenceTest(unittest.TestCase):
    def test_openai_transcription_recognizes_known_russian_words(self):
        from fastapi.testclient import TestClient

        from asr_service.main import app

        source = Path(os.environ["ASR_REAL_TONE_AUDIO"])
        self.assertTrue(source.is_file(), "synthetic WAV fixture must exist")
        with TestClient(app) as client, source.open("rb") as recording:
            response = client.post(
                "/v1/audio/transcriptions",
                files={"file": ("synthetic-russian.wav", recording, "audio/wav")},
                data={"model": "t-one", "language": "ru"},
            )
        self.assertEqual(response.status_code, 200, response.text)
        payload = response.json()
        self.assertIsInstance(payload.get("text"), str)
        transcript = " ".join(re.findall(r"[а-яё]+", payload["text"].lower())).replace("ё", "е")
        self.assertEqual(transcript, EXPECTED_TRANSCRIPT)
        print(json.dumps({"expected": EXPECTED_TRANSCRIPT, "actual": payload["text"], "normalized": transcript}, ensure_ascii=False))


@unittest.skipUnless(os.environ.get("ASR_REAL_TONE_LONG_AUDIO"), "real cancellation lane requires a long synthetic audio fixture")
class RealToneCancellationTest(unittest.TestCase):
    def test_cancel_requested_during_real_inference_wins_over_done(self):
        from fastapi.testclient import TestClient

        from asr_service.main import app

        source = Path(os.environ["ASR_REAL_TONE_LONG_AUDIO"])
        self.assertTrue(source.is_file(), "long synthetic WAV fixture must exist")
        with TestClient(app) as client, source.open("rb") as recording:
            uploaded = client.post("/v1/asr/uploads", files={"file": ("synthetic-russian-long.wav", recording, "audio/wav")})
            self.assertEqual(uploaded.status_code, 201, uploaded.text)
            models = client.get("/v1/asr/models")
            self.assertEqual(models.status_code, 200, models.text)
            payload = models.json()
            rows = payload.get("models", payload) if isinstance(payload, dict) else payload
            descriptor = next((row for row in rows if isinstance(row, dict) and (row.get("available", True) or row.get("status") in {"ready", "available"})), rows[0])
            model_key = descriptor.get("key") or descriptor.get("model_key") or descriptor.get("id") or descriptor.get("name")
            created = client.post(
                "/v1/asr/jobs",
                json={"model_key": model_key, "language": "ru", "source_uri": uploaded.json()["source_uri"]},
            )
            self.assertEqual(created.status_code, 201, created.text)
            job_id = created.json()["job_id"]
            canceled = client.delete(f"/v1/asr/jobs/{job_id}")
            self.assertEqual(canceled.status_code, 200, canceled.text)
            deadline = time.monotonic() + 180
            current = canceled.json()
            while current.get("status") not in {"canceled", "done", "error", "expired"} and time.monotonic() < deadline:
                time.sleep(0.25)
                current = client.get(f"/v1/asr/jobs/{job_id}").json()
        self.assertEqual(current.get("status"), "canceled", current)
        self.assertTrue(current.get("cancel_requested"), current)
        self.assertIsNone(current.get("result"), current)
        self.assertFalse(current.get("artifacts"), current)


if __name__ == "__main__":
    unittest.main()
