# ragflow-sdk

# build and publish python SDK to pypi.org

```shell
uv build
uv pip install twine
export TWINE_USERNAME="__token__"
export TWINE_PASSWORD=$YOUR_PYPI_API_TOKEN
twine upload dist/*.whl
```
