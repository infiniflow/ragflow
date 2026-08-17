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
"""Regression tests for the ``StandardizeImage`` operator in
``deepdoc/vision/operators.py``.

Issue: infiniflow/ragflow#7316.

The class was defined as ``StandardizeImag`` (typo, missing the final ``e``)
but ``deepdoc/vision/recognizer.py`` dispatches preprocessing ops via::

    op_type = new_op_info.pop("type")  # "StandardizeImage"
    preprocess_ops.append(getattr(operators, op_type)(**new_op_info))

so ``getattr(operators, "StandardizeImage")`` raised ``AttributeError`` and the
standardize step silently never ran. The fix renames the class to match the
canonical name used by every caller.

These tests pin both contracts:

1. ``deepdoc.vision.operators`` exposes the class under its canonical name
   (``StandardizeImage``), which is the name the recognizer looks up.
2. The operator performs the documented mean/std normalization.
"""

import importlib.util
import os
import sys
from types import ModuleType

import pytest


# Names of the real project-internal modules that the operators.py loader
# stubs out so we don't need the full RAGFlow runtime. The fixture below
# records the pre-test value of each entry in ``sys.modules`` and restores
# it on teardown, so neighbouring tests that legitimately import these
# modules are never silently handed the stub.
_STUB_MODULE_NAMES = ("rag", "rag.utils", "rag.utils.lazy_image")


@pytest.fixture
def operators_module():
    """Load ``deepdoc.vision.operators`` from source with stubbed project
    imports, and clean the stubs up afterwards.

    The fixture records every ``sys.modules`` entry it touches and restores
    the pre-test state in the teardown phase, so a later test that imports
    the real ``rag.utils.lazy_image`` continues to receive the real module
    rather than the identity-pass-through stub installed here.
    """
    project_root = os.path.abspath(
        os.path.join(os.path.dirname(__file__), "..", "..", "..", "..")
    )

    # Snapshot the entries we'll mutate so teardown can restore them
    # exactly, even when some of them were already populated.
    snapshot = {name: sys.modules.get(name) for name in _STUB_MODULE_NAMES}

    rag_pkg = sys.modules.setdefault(
        "rag", ModuleType("rag"),
    )
    rag_pkg.__path__ = [os.path.join(project_root, "rag")]

    rag_utils = sys.modules.setdefault(
        "rag.utils", ModuleType("rag.utils"),
    )
    rag_utils.__path__ = [os.path.join(project_root, "rag", "utils")]

    lazy_image = ModuleType("rag.utils.lazy_image")
    lazy_image.ensure_pil_image = lambda im: im
    sys.modules["rag.utils.lazy_image"] = lazy_image

    operators_path = os.path.join(
        project_root, "deepdoc", "vision", "operators.py"
    )
    spec = importlib.util.spec_from_file_location(
        "_test_operators_under_test", operators_path
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    try:
        yield module
    finally:
        # Restore the exact pre-test state. We delete first so a sibling
        # module that did ``import rag`` mid-test doesn't get a hidden
        # half-populated stub left over.
        for name in _STUB_MODULE_NAMES:
            if name in sys.modules and sys.modules[name] is snapshot.get(name):
                continue
            if snapshot[name] is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = snapshot[name]


def test_standardize_image_class_resolves_by_canonical_name(operators_module):
    """Regression for #7316.

    The recognizer dispatches preprocessing ops by their string ``"type"``
    key (see ``deepdoc/vision/recognizer.py`` ``preprocess()``), and the
    canonical name it uses is ``"StandardizeImage"``. The class must be
    importable from ``deepdoc.vision.operators`` under that name so
    ``getattr(operators, "StandardizeImage")`` succeeds.
    """
    assert hasattr(operators_module, "StandardizeImage"), (
        "deepdoc.vision.operators must expose a 'StandardizeImage' class — "
        "recognizer.py dispatches preprocessing ops by this name; a typo in "
        "the class name causes AttributeError at runtime."
    )
    assert isinstance(operators_module.StandardizeImage, type), (
        "StandardizeImage must be a class."
    )


def test_standardize_image_callable_matches_legacy_alias_name(operators_module):
    """The previously-broken alias ``StandardizeImag`` must no longer be
    available — the typo is gone. If a downstream caller ever relied on the
    misnamed class, this test will fail loudly so we can decide whether to
    re-add a compatibility shim.
    """
    assert not hasattr(operators_module, "StandardizeImag"), (
        "The misspelled 'StandardizeImag' class name should have been "
        "removed; if something still references it, add a compatibility "
        "shim and revisit this assertion."
    )


def test_standardize_image_normalizes_input_with_mean_std_and_is_scale(operators_module):
    """End-to-end behavior: with is_scale=True, mean_std, the operator must
    divide by 255 and then subtract mean / divide by std (per the class
    docstring).
    """
    import numpy as np

    op = operators_module.StandardizeImage(
        mean=[0.5, 0.5, 0.5],
        std=[0.5, 0.5, 0.5],
        is_scale=True,
        norm_type="mean_std",
    )

    # A 1x1x3 image with all-ones in the range [0, 255].
    im = np.array([[[255.0, 255.0, 255.0]]], dtype=np.float32)
    im_info = {}

    out_im, out_info = op(im, im_info)

    # After /255 -> 1.0; (1.0 - 0.5) / 0.5 = 1.0
    assert out_im.shape == im.shape
    assert np.allclose(out_im, [[[1.0, 1.0, 1.0]]]), (
        f"Expected mean-std normalized output of [[[1,1,1]]], got {out_im!r}"
    )
    # im_info is passed through unchanged.
    assert out_info is im_info


def test_standardize_image_skips_scaling_when_is_scale_false(operators_module):
    """When is_scale=False, the operator must NOT divide by 255 before
    applying mean/std.
    """
    import numpy as np

    op = operators_module.StandardizeImage(
        mean=[1.0, 1.0, 1.0],
        std=[2.0, 2.0, 2.0],
        is_scale=False,
        norm_type="mean_std",
    )

    # A 1x1x3 image with values 9, 9, 9.
    im = np.array([[[9.0, 9.0, 9.0]]], dtype=np.float32)
    out_im, _ = op(im, {})

    # No /255; (9 - 1) / 2 = 4
    assert np.allclose(out_im, [[[4.0, 4.0, 4.0]]]), (
        f"Expected is_scale=False path to skip /255, got {out_im!r}"
    )


def test_standardize_image_norm_type_none_passes_image_through(operators_module):
    """With norm_type='none' the operator must not subtract mean or divide by
    std. is_scale is still applied if True.
    """
    import numpy as np

    op = operators_module.StandardizeImage(
        mean=[123.0, 456.0, 789.0],  # values that would corrupt the output
        std=[1.0, 1.0, 1.0],
        is_scale=True,
        norm_type="none",
    )

    im = np.array([[[255.0, 255.0, 255.0]]], dtype=np.float32)
    out_im, _ = op(im, {})

    # /255 = 1.0; no mean/std applied.
    assert np.allclose(out_im, [[[1.0, 1.0, 1.0]]]), (
        f"Expected norm_type='none' to skip mean/std, got {out_im!r}"
    )


def test_standardize_image_via_module_getattr_dispatch_path(operators_module):
    """Mirrors the exact dispatch path used by
    ``deepdoc/vision/recognizer.py:preprocess()``::

        op_type = new_op_info.pop("type")
        preprocess_ops.append(getattr(operators, op_type)(**new_op_info))

    so any future regression in the class name will fail this test as
    ``AttributeError`` rather than silently producing broken preprocessing.
    """
    import numpy as np

    op_info = {
        "is_scale": True,
        "mean": [0.5, 0.5, 0.5],
        "std": [0.5, 0.5, 0.5],
        "type": "StandardizeImage",
    }
    new_op_info = op_info.copy()
    op_type = new_op_info.pop("type")

    # This is the exact line from recognizer.py; if it raises AttributeError
    # the bug is back.
    op = getattr(operators_module, op_type)(**new_op_info)

    im = np.array([[[255.0, 255.0, 255.0]]], dtype=np.float32)
    out_im, _ = op(im, {})

    assert np.allclose(out_im, [[[1.0, 1.0, 1.0]]])
