"""Opt-in real T-one API contract using a known synthetic Russian recording.

Run as unittest in an isolated service environment with ASR_REAL_TONE_AUDIO set
to a WAV speaking EXPECTED_TRANSCRIPT. No engine, preprocessing or HTTP mocks.
The normal unit suite skips this lane when the real audio fixture is absent.
"""

import json
import os
from pathlib import Path
import re
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


if __name__ == "__main__":
    unittest.main()
