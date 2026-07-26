#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Regression tests for split_model_name() in api.db.joint_services.tenant_model_service.

The composite model key format used by RAGFlow is right-anchored:
``model_name@instance_name@provider_name`` for a manual instance, or
``model_name@provider_name`` when the instance defaults to ``"default"``.

Some model names legitimately contain ``@`` characters (for example LM
Studio embedding IDs like ``text-embedding-nomic-embed-text-v1.5@q8_0``),
producing four-``@``-separated composite keys such as
``text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio``.

A naive ``split("@")`` would mis-parse these keys, stripping the
quantization suffix off the model name and treating the wrong middle field
as the provider. The function uses ``rsplit("@", 2)`` so the rightmost two
fields remain the instance and provider regardless of how many ``@``
characters appear in the leftmost model-name field.

This test loads only the function definition from the source file via AST,
so it doesn't pull in the full ``common.settings`` import chain (which
transitively imports ``tiktoken``, ``onnxruntime``, the ES connector, etc.)
and can run in any minimal pytest environment.
"""

import ast
from pathlib import Path

import pytest

pytestmark = pytest.mark.p2


def _load_split_model_name():
    """Load the `split_model_name` function from the production source file.

    The function is pure-Python and does not reference any module-level
    names, so it can be exec'd standalone in an empty namespace.
    """
    src_path = (
        Path(__file__).resolve().parents[5]
        / "api"
        / "db"
        / "joint_services"
        / "tenant_model_service.py"
    )
    tree = ast.parse(src_path.read_text(encoding="utf-8"))
    fn_node = next(
        node for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name == "split_model_name"
    )
    module = ast.Module(body=[fn_node], type_ignores=[])
    ns: dict = {}
    exec(compile(module, str(src_path), "exec"), ns)
    return ns["split_model_name"]


split_model_name = _load_split_model_name()


@pytest.mark.p2
def test_split_model_name_with_at_symbol_in_model_name_lm_studio_embedding():
    """The exact LM Studio case from the dataset-configuration bug.

    Before the fix, this raised nothing but returned the wrong
    ``provider_name='lmstudio'``, triggering
    ``LookupError("Provider lmstudio not found ...")``. After the rsplit
    fix, the full ``@q8_0`` suffix stays attached to ``model_name`` and
    ``provider_name`` correctly resolves to ``LM-Studio``.
    """
    composite = "text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio"
    pure, instance, provider = split_model_name(composite)

    assert pure == "text-embedding-nomic-embed-text-v1.5@q8_0"
    assert instance == "lmstudio"
    assert provider == "LM-Studio"


@pytest.mark.p2
def test_split_model_name_plain_three_part():
    """Standard 3-part key: model@instance@provider."""
    pure, instance, provider = split_model_name("gpt-4o@eastus@OpenAI")

    assert pure == "gpt-4o"
    assert instance == "eastus"
    assert provider == "OpenAI"


@pytest.mark.p2
def test_split_model_name_two_part_defaults_instance():
    """Two-part key (no explicit instance) falls back to ``default``."""
    pure, instance, provider = split_model_name("gpt-4o@OpenAI")

    assert pure == "gpt-4o"
    assert instance == "default"
    assert provider == "OpenAI"


@pytest.mark.p2
def test_split_model_name_bare_model_name():
    """Bare model name (no '@' anywhere) yields empty provider/instance."""
    pure, instance, provider = split_model_name("qwen2.5-7b-instruct")

    assert pure == "qwen2.5-7b-instruct"
    assert instance == ""
    assert provider == ""


@pytest.mark.p2
def test_split_model_name_multiple_at_symbols_in_model_name():
    """Several embedded ``@`` characters in the model name are preserved."""
    composite = "foo@bar@baz@q4_k_m@myinst@MyProvider"
    pure, instance, provider = split_model_name(composite)

    assert pure == "foo@bar@baz@q4_k_m"
    assert instance == "myinst"
    assert provider == "MyProvider"


@pytest.mark.p2
def test_split_model_name_three_part_with_at_in_model_name():
    """3-part key where the model name contains a single ``@``.

    Composite: ``foo@bar@HuggingFace`` parses as a 3-part key
    (``foo``, ``bar``, ``HuggingFace``). The middle field is treated as
    the explicit instance. This is the smallest case that exercises the
    rsplit behavior on a model name containing ``@``.
    """
    pure, instance, provider = split_model_name("foo@bar@HuggingFace")

    assert pure == "foo"
    assert instance == "bar"
    assert provider == "HuggingFace"


@pytest.mark.p2
def test_split_model_name_returns_tuple_of_strings():
    """Sanity: the function must always return three string values."""
    pure, instance, provider = split_model_name("a@b@c")
    assert isinstance(pure, str)
    assert isinstance(instance, str)
    assert isinstance(provider, str)