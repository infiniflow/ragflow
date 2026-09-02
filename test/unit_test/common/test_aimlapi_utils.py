from common.aimlapi_utils import DEFAULT_PARTNER_ID, DEFAULT_SOURCE, attribution_headers, partner_id, source


def test_defaults(monkeypatch):
    monkeypatch.delenv("AIMLAPI_SOURCE", raising=False)
    monkeypatch.delenv("AIMLAPI_PARTNER_ID", raising=False)

    assert source() == DEFAULT_SOURCE
    assert attribution_headers() == {
        "X-AIMLAPI-Source": DEFAULT_SOURCE,
        "X-AIMLAPI-Partner-ID": DEFAULT_PARTNER_ID,
    }


def test_environment_overrides(monkeypatch):
    monkeypatch.setenv("AIMLAPI_SOURCE", " agent/custom ")
    monkeypatch.setenv("AIMLAPI_PARTNER_ID", " part_custom ")

    assert source() == "agent/custom"
    assert partner_id() == "part_custom"
    assert attribution_headers() == {
        "X-AIMLAPI-Source": "agent/custom",
        "X-AIMLAPI-Partner-ID": "part_custom",
    }


def test_blank_partner_id_is_omitted(monkeypatch):
    monkeypatch.delenv("AIMLAPI_SOURCE", raising=False)
    monkeypatch.setenv("AIMLAPI_PARTNER_ID", "  ")

    headers = attribution_headers()
    assert headers == {"X-AIMLAPI-Source": DEFAULT_SOURCE}
