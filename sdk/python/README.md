# python-ragflow

# update python client

- Update "version" field of [project] chapter
- Build new python SDK
- Upload to pypi.org
- Install new python SDK

# build python SDK

```shell
rm -f dist/* && python setup.py sdist bdist_wheel
```

# install python SDK
```shell
pip uninstall -y ragflow && pip install dist/*.whl
```

This will install ragflow-sdk and its dependencies.

# upload to pypi.org
```shell
twine upload dist/*.whl
```

Enter your pypi API token according to the prompt.

Note that pypi allows a version of a package [be uploaded only once](https://pypi.org/help/#file-name-reuse). You need to change the `version` inside the `pyproject.toml` before building and uploading.

# using

```python

```

# For developer
```shell
pip install -e .
```
