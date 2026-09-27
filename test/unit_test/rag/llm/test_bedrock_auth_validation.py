#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

"""Cycle 84 regression coverage for ``BedrockEmbed`` auth-mode validation.

The cycle 84 fix adds explicit guards in the ``access_key_secret`` and
``iam_role`` branches of ``BedrockEmbed.__init__`` so operators get a
clear, actionable error instead of an opaque boto3 ``ParamValidationError``
or ``KeyError`` when the key is missing the fields that branch requires.

The ``rag/llm/`` import chain pulls in the C++ ``infinity.rag_tokenizer``
static library, which is not built in the local dev environment (CI runs
the full matrix). To keep these tests runnable locally, they use source-
string inspection of ``rag/llm/embedding_model.py`` rather than importing
the module. CI's full test environment runs the same tests against the
live ``BedrockEmbed`` code path; this source-level suite pins the guards
so a regression cannot slip past review.
"""

import re
from pathlib import Path


EMBEDDING_MODEL_PATH = Path("rag/llm/embedding_model.py")


def _source() -> str:
    return EMBEDDING_MODEL_PATH.read_text(encoding="utf-8")


def _bedrock_init_block() -> str:
    """Return the body of ``BedrockEmbed.__init__`` as source text."""
    src = _source()
    match = re.search(
        r"class BedrockEmbed\(Base\):\s*\n\s*_FACTORY_NAME = \"Bedrock\"\s*\n\s*def __init__\(self, key, model_name, \*\*kwargs\):(.*?)(?=\nclass |\n    def |\Z)",
        src,
        re.DOTALL,
    )
    assert match, "BedrockEmbed.__init__ not found in embedding_model.py"
    return match.group(1)


class TestAccessKeySecretGuards:
    """Pin that ``access_key_secret`` rejects a missing ak/sk."""

    def setup_method(self):
        self.body = _bedrock_init_block()

    def test_branch_raises_on_missing_ak_or_sk(self):
        match = re.search(
            r"if not self\.bedrock_ak or not self\.bedrock_sk:\s*\n\s*raise ValueError\(\s*\n?\s*f?[\"']Bedrock access_key_secret mode requires both bedrock_ak and bedrock_sk",
            self.body,
        )
        assert match, "access_key_secret branch must raise ValueError with a message that names the mode and both fields when bedrock_ak or bedrock_sk is empty"

    def test_guard_precedes_boto3_client_call(self):
        guard_pos = self.body.find("if not self.bedrock_ak or not self.bedrock_sk")
        client_pos = self.body.find('boto3.client(service_name="bedrock-runtime"', guard_pos)
        assert guard_pos != -1 and client_pos != -1 and guard_pos < client_pos, "access_key_secret guard must precede the boto3.client call"


class TestIamRoleGuards:
    """Pin that ``iam_role`` rejects a missing role ARN and surfaces clear errors."""

    def setup_method(self):
        self.body = _bedrock_init_block()

    def test_branch_raises_on_missing_role_arn(self):
        match = re.search(
            r"if not self\.aws_role_arn:\s*\n\s*raise ValueError\(\s*\n?\s*f?[\"']Bedrock iam_role mode requires aws_role_arn",
            self.body,
        )
        assert match, "iam_role branch must raise ValueError with a message that names the mode and the required field when aws_role_arn is empty"

    def test_branch_wraps_assume_role_exception(self):
        match = re.search(
            r"try:\s*\n\s*resp = sts_client\.assume_role\(",
            self.body,
        )
        assert match, "iam_role branch must wrap sts_client.assume_role in a try/except"

        wrap_msg = re.search(
            r"raise RuntimeError\(\s*\n?\s*f[\"']Bedrock iam_role assume_role failed for role",
            self.body,
        )
        assert wrap_msg, "iam_role branch must re-raise a RuntimeError that names the role and region so operators can distinguish 'config missing' from 'AWS denied AssumeRole' from 'network blip'"

    def test_branch_validates_credentials_in_response(self):
        match = re.search(
            r"creds = resp\.get\([\"']Credentials[\"']\)\s*\n\s*if not creds:\s*\n\s*raise RuntimeError\(\s*\n?\s*f[\"']Bedrock iam_role assume_role returned no Credentials",
            self.body,
        )
        assert match, "iam_role branch must use resp.get('Credentials') and raise a RuntimeError with role context when the response lacks Credentials"

    def test_credential_index_access_still_present(self):
        # After the validation guard, the three key indexes must still be there
        # — they are the live access path. Source uses double quotes; normalise
        # via regex to accept either quote style.
        for key in ("AccessKeyId", "SecretAccessKey", "SessionToken"):
            assert re.search(rf"creds\[['\"]{key}['\"]\]", self.body), f"creds['{key}'] index access must remain after the validation guard"


class TestGuardOrdering:
    """Pin the relative order of the new guards so they cannot be reordered."""

    def setup_method(self):
        self.body = _bedrock_init_block()

    def test_access_key_secret_guard_precedes_iam_role_guard(self):
        ak_guard = self.body.find("if not self.bedrock_ak or not self.bedrock_sk")
        iam_guard = self.body.find("if not self.aws_role_arn")
        assert ak_guard != -1 and iam_guard != -1 and ak_guard < iam_guard, "access_key_secret guard must come before iam_role guard"

    def test_iam_role_arn_guard_precedes_assume_role_call(self):
        guard = self.body.find("if not self.aws_role_arn")
        assume_role = self.body.find("sts_client.assume_role(", guard)
        assert guard != -1 and assume_role != -1 and guard < assume_role, "iam_role role_arn guard must precede the assume_role call"

    def test_assume_role_try_block_precedes_credentials_validation(self):
        # The try/except wrapping the assume_role call must come before the
        # resp.get('Credentials') guard so the network/permission failure
        # path is captured before the dict-access path.
        try_pos = self.body.find("try:\n")
        try_pos = self.body.find("sts_client.assume_role(", try_pos)
        guard_match = re.search(r'resp\.get\([\'"]Credentials[\'"]\)', self.body)
        assert try_pos != -1 and guard_match is not None and try_pos < guard_match.start(), "assume_role try/except must precede the resp.get('Credentials') guard"
