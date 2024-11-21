from ..common import HOST_ADDRESS
import requests
def test_create_dataset(get_auth):
    authorization={"Authorization": get_auth}
    url = f"{HOST_ADDRESS}/v1/kb/create"
    json = {"name":"test_create_dataset"}
    res = requests.post(url=url,headers=authorization,json=json)
    res = res.json()
    assert res.get("code") == 0,f"{res.get('message')}"

