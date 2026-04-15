#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import pytest
from jinja2.exceptions import SecurityError, UndefinedError
from jinja2.sandbox import SandboxedEnvironment

from rag.prompts.generator import PROMPT_JINJA_ENV


@pytest.mark.p1
class TestJinjaSandbox:
    """Test that PROMPT_JINJA_ENV uses SandboxedEnvironment to prevent SSTI attacks."""

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload",
        [
            # Classic SSTI payloads targeting __globals__, __mro__, __subclasses__
            "{{ self.__class__.__mro__[1].__subclasses__() }}",
            "{{ ''.__class__.__mro__[1].__subclasses__() }}",
            "{{ request.__class__.__mro__[1].__subclasses__() }}",
            # Attribute traversal (no hardcoded subclass index)
            "{{ config.__class__.__init__.__globals__['os'] }}",
        ],
    )
    def test_ssti_payload_blocked(self, payload):
        """Verify that SSTI payloads are blocked by SandboxedEnvironment."""
        assert isinstance(PROMPT_JINJA_ENV, SandboxedEnvironment), (
            "PROMPT_JINJA_ENV must use SandboxedEnvironment to prevent SSTI"
        )
        template = PROMPT_JINJA_ENV.from_string(payload)
        # SandboxedEnvironment raises SecurityError, AttributeError, or UndefinedError to block SSTI attacks
        with pytest.raises((SecurityError, AttributeError, UndefinedError)) as exc_info:
            template.render()
        # Verify exception contains sandbox indicators
        exc_msg = str(exc_info.value)
        assert any(x in exc_msg.lower() for x in ["unsafe", "security", "__mro__"])

    @pytest.mark.p1
    def test_safe_template_rendering(self):
        """Verify that benign templates still render correctly."""
        template = PROMPT_JINJA_ENV.from_string("Hello, {{ name }}!")
        result = template.render(name="World")
        assert result == "Hello, World!"

    @pytest.mark.p1
    def test_loop_and_conditional_rendering(self):
        """Verify control flow templates work properly."""
        template = PROMPT_JINJA_ENV.from_string(
            "{% for item in items %}{{ item }}{% endfor %}"
        )
        result = template.render(items=["a", "b", "c"])
        assert result == "abc"
