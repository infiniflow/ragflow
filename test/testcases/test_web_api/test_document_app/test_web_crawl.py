import requests

import pytest
from common import DOCUMENT_APP_URL, HOST_ADDRESS


def _web_crawl(auth, payload):
    url = f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/web_crawl"
    res = requests.post(url=url, auth=auth, data=payload)
    return res.json()


@pytest.mark.p2
def test_web_crawl_validation_errors(WebApiAuth, add_dataset_func):
    kb_id = add_dataset_func

    res = _web_crawl(WebApiAuth, {"name": "doc", "url": "https://www.example.com"})
    assert res["code"] == 101, res
    assert "kb_id" in res["message"].lower(), res

    res = _web_crawl(WebApiAuth, {"kb_id": kb_id, "name": "doc", "url": "bad"})
    assert res["code"] == 101, res
    assert "URL" in res["message"], res

    res = _web_crawl(WebApiAuth, {"kb_id": "invalid_kb_id", "name": "doc", "url": "http://8.8.8.8"})
    assert res["code"] != 0, res
    assert "dataset" in res.get("message", ""), res
