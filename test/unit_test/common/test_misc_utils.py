#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import asyncio
import contextvars
import hashlib
import sys
import types
import uuid
from contextlib import contextmanager
from unittest.mock import patch

import pytest

from common import ssrf_guard
from common.misc_utils import convert_bytes, download_img, get_uuid, hash_str2int, thread_pool_exec


class _Hdr:
    def __init__(self, mapping: dict[str, str]):
        self._m = {k.lower(): v for k, v in mapping.items()}

    def get(self, key: str, default=None):
        return self._m.get(key.lower(), default)


class _MockStreamResp:
    def __init__(self, status_code: int, *, location: str | None = None, body: bytes = b""):
        self.status_code = status_code
        hdrs: dict[str, str] = {}
        if location is not None:
            hdrs["Location"] = location
        if body:
            hdrs.setdefault("Content-Type", "image/jpeg")
        self.headers = _Hdr(hdrs)
        self._body = body

    async def aclose(self):
        return None

    async def aiter_bytes(self):
        if self._body:
            yield self._body


class _FakeStreamCtx:
    def __init__(self, resp: _MockStreamResp):
        self._resp = resp

    async def __aenter__(self):
        return self._resp

    async def __aexit__(self, exc_type, exc, tb):
        return None


class _FakeAsyncClient:
    def __init__(self, responses: list[_MockStreamResp]):
        self._responses = responses

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc, tb):
        return None

    def stream(self, method, url, headers=None):
        if not self._responses:
            return _FakeStreamCtx(_MockStreamResp(404))
        return _FakeStreamCtx(self._responses.pop(0))


@contextmanager
def _fake_httpx_sys_modules(client):
    """Minimal ``httpx`` stub so ``download_img`` can be exercised without real httpx."""
    saved = sys.modules.get("httpx")
    fake = types.ModuleType("httpx")

    class _Timeout:
        def __init__(self, *_a, **_kw):
            pass

    fake.Timeout = _Timeout

    def _AsyncClient(*_a, **_kw):
        return client

    fake.AsyncClient = _AsyncClient
    sys.modules["httpx"] = fake
    try:
        yield
    finally:
        if saved is not None:
            sys.modules["httpx"] = saved
        else:
            sys.modules.pop("httpx", None)


@pytest.mark.p1
class TestThreadPoolExec:
    """Test cases for thread_pool_exec — verifies ContextVar propagation into the worker thread."""

    def test_contextvar_propagated_to_thread(self):
        """ContextVar set in async caller must be visible inside the thread."""
        _var: contextvars.ContextVar[str] = contextvars.ContextVar("_var")

        def read_var():
            return _var.get(None)

        async def run():
            _var.set("hello")
            return await thread_pool_exec(read_var)

        result = asyncio.run(run())
        assert result == "hello"

    def test_contextvar_propagated_with_kwargs(self):
        """ContextVar propagation also works when kwargs are passed (functools.partial path)."""
        _var: contextvars.ContextVar[int] = contextvars.ContextVar("_var_kw")

        def read_var_and_add(increment):
            return (_var.get(0)) + increment

        async def run():
            _var.set(10)
            return await thread_pool_exec(read_var_and_add, increment=5)

        result = asyncio.run(run())
        assert result == 15

    def test_contextvar_isolation_between_calls(self):
        """Each thread_pool_exec call captures the context at submission time."""
        _var: contextvars.ContextVar[str] = contextvars.ContextVar("_var_iso")

        def read_var():
            return _var.get(None)

        async def run():
            _var.set("first")
            r1 = await thread_pool_exec(read_var)
            _var.set("second")
            r2 = await thread_pool_exec(read_var)
            return r1, r2

        r1, r2 = asyncio.run(run())
        assert r1 == "first"
        assert r2 == "second"

    def test_unset_contextvar_returns_default(self):
        """A ContextVar that was never set in caller returns its default inside the thread."""
        _var: contextvars.ContextVar[str] = contextvars.ContextVar("_var_unset", default="default")

        def read_var():
            return _var.get()

        result = asyncio.run(thread_pool_exec(read_var))
        assert result == "default"


class TestGetUuid:
    """Test cases for get_uuid function"""

    def test_returns_string(self):
        """Test that function returns a string"""
        result = get_uuid()
        assert isinstance(result, str)

    def test_hex_format(self):
        """Test that returned string is in hex format"""
        result = get_uuid()
        # UUID v1 hex should be 32 characters (without dashes)
        assert len(result) == 32
        # Should only contain hexadecimal characters
        assert all(c in "0123456789abcdef" for c in result)

    def test_no_dashes_in_result(self):
        """Test that result contains no dashes"""
        result = get_uuid()
        assert "-" not in result

    def test_unique_results(self):
        """Test that multiple calls return different UUIDs"""
        results = [get_uuid() for _ in range(10)]

        # All results should be unique
        assert len(results) == len(set(results))

        # All should be valid hex strings of correct length
        for result in results:
            assert len(result) == 32
            assert all(c in "0123456789abcdef" for c in result)

    def test_valid_uuid_structure(self):
        """Test that the hex string can be converted back to UUID"""
        result = get_uuid()

        # Should be able to create UUID from the hex string
        reconstructed_uuid = uuid.UUID(hex=result)
        assert isinstance(reconstructed_uuid, uuid.UUID)

        # The hex representation should match the original
        assert reconstructed_uuid.hex == result

    def test_uuid1_specific_characteristics(self):
        """Test that UUID v1 characteristics are present"""
        result = get_uuid()
        uuid_obj = uuid.UUID(hex=result)

        # UUID v1 should have version 1
        assert uuid_obj.version == 1

        # Variant should be RFC 4122
        assert uuid_obj.variant == "specified in RFC 4122"

    def test_result_length_consistency(self):
        """Test that all generated UUIDs have consistent length"""
        for _ in range(100):
            result = get_uuid()
            assert len(result) == 32

    def test_hex_characters_only(self):
        """Test that only valid hex characters are used"""
        for _ in range(100):
            result = get_uuid()
            # Should only contain lowercase hex characters (UUID hex is lowercase)
            assert result.islower()
            assert all(c in "0123456789abcdef" for c in result)


class TestDownloadImg:
    """Test cases for download_img function"""

    def test_empty_url_returns_empty_string(self):
        """Test that empty URL returns empty string"""
        result = asyncio.run(download_img(""))
        assert result == ""

    def test_none_url_returns_empty_string(self):
        """Test that None URL returns empty string"""
        result = asyncio.run(download_img(None))
        assert result == ""

    def test_loopback_url_blocked(self):
        """OAuth avatar fetch must not call loopback (SSRF regression)."""
        result = asyncio.run(download_img("http://127.0.0.1/avatar.png"))
        assert result == ""

    def test_metadata_ip_blocked(self):
        """Link-local / cloud metadata ranges are non-global and must be rejected."""
        result = asyncio.run(download_img("http://169.254.169.254/latest/meta-data/"))
        assert result == ""

    def test_disallowed_scheme_blocked(self):
        result = asyncio.run(download_img("file:///etc/passwd"))
        assert result == ""

    def test_redirect_to_loopback_blocked(self):
        """Redirect from an allowed host to loopback must be rejected (SSRF)."""
        from urllib.parse import urlparse

        real_assert = ssrf_guard.assert_url_is_safe

        def selective_assert(url: str, **kwargs):
            host = urlparse(url).hostname or ""
            if host in ("127.0.0.1", "localhost", "169.254.169.254"):
                return real_assert(url, **kwargs)
            return ("public-avatar.test", "8.8.8.8")

        client = _FakeAsyncClient([_MockStreamResp(302, location="http://127.0.0.1/next")])

        with (
            patch.object(ssrf_guard, "assert_url_is_safe", side_effect=selective_assert),
            _fake_httpx_sys_modules(client),
        ):
            result = asyncio.run(download_img("http://public-avatar.test/start.png"))
        assert result == ""

    def test_redirect_too_many_hops_blocked(self):
        """Excessive redirect chains must return empty without hanging."""
        import common.misc_utils as misc_utils

        hops = [
            _MockStreamResp(302, location="http://h.example/1"),
            _MockStreamResp(302, location="http://h.example/2"),
            _MockStreamResp(302, location="http://h.example/3"),
        ]
        client = _FakeAsyncClient(hops)

        with (
            patch.object(misc_utils, "_OAUTH_AVATAR_MAX_REDIRECTS", 2),
            patch.object(ssrf_guard, "assert_url_is_safe", return_value=("h.example", "8.8.8.8")),
            _fake_httpx_sys_modules(client),
        ):
            result = asyncio.run(misc_utils.download_img("http://h.example/start"))
        assert result == ""


class TestHashStr2Int:
    """Test cases for hash_str2int function"""

    def test_basic_hashing(self):
        """Test basic string hashing functionality"""
        result = hash_str2int("hello")
        assert isinstance(result, int)
        assert 0 <= result < 10**8

    def test_default_mod_value(self):
        """Test that default mod value is 10^8"""
        result = hash_str2int("test")
        assert 0 <= result < 10**8

    def test_custom_mod_value(self):
        """Test with custom mod value"""
        result = hash_str2int("test", mod=1000)
        assert isinstance(result, int)
        assert 0 <= result < 1000

    def test_same_input_same_output(self):
        """Test that same input produces same output"""
        result1 = hash_str2int("consistent")
        result2 = hash_str2int("consistent")
        result3 = hash_str2int("consistent")

        assert result1 == result2 == result3

    def test_different_input_different_output(self):
        """Test that different inputs produce different outputs (usually)"""
        result1 = hash_str2int("hello")
        result2 = hash_str2int("world")
        result3 = hash_str2int("hello world")

        # While hash collisions are possible, they're very unlikely for these inputs
        results = [result1, result2, result3]
        assert len(set(results)) == len(results)

    def test_empty_string(self):
        """Test hashing empty string"""
        result = hash_str2int("")
        assert isinstance(result, int)
        assert 0 <= result < 10**8

    def test_unicode_string(self):
        """Test hashing unicode strings"""
        test_strings = ["中文", "🚀火箭", "café", "🎉", "Hello 世界"]

        for test_str in test_strings:
            result = hash_str2int(test_str)
            assert isinstance(result, int)
            assert 0 <= result < 10**8

    def test_special_characters(self):
        """Test hashing strings with special characters"""
        test_strings = ["hello@world.com", "test#123", "line\nwith\nnewlines", "tab\tcharacter", "space in string"]

        for test_str in test_strings:
            result = hash_str2int(test_str)
            assert isinstance(result, int)
            assert 0 <= result < 10**8

    def test_large_string(self):
        """Test hashing large string"""
        large_string = "x" * 10000
        result = hash_str2int(large_string)
        assert isinstance(result, int)
        assert 0 <= result < 10**8

    def test_mod_value_1(self):
        """Test with mod value 1 (should always return 0)"""
        result = hash_str2int("any string", mod=1)
        assert result == 0

    def test_mod_value_2(self):
        """Test with mod value 2 (should return 0 or 1)"""
        result = hash_str2int("test", mod=2)
        assert result in [0, 1]

    def test_very_large_mod(self):
        """Test with very large mod value"""
        result = hash_str2int("test", mod=10**12)
        assert isinstance(result, int)
        assert 0 <= result < 10**12

    def test_hash_algorithm_sha1(self):
        """Test that SHA1 algorithm is used"""
        test_string = "hello"
        expected_hash = hashlib.sha1(test_string.encode("utf-8")).hexdigest()
        expected_int = int(expected_hash, 16) % (10**8)

        result = hash_str2int(test_string)
        assert result == expected_int

    def test_utf8_encoding(self):
        """Test that UTF-8 encoding is used"""
        # This should work without encoding errors
        result = hash_str2int("café 🎉")
        assert isinstance(result, int)

    def test_range_with_different_mods(self):
        """Test that result is always in correct range for different mod values"""
        test_cases = [
            ("test1", 100),
            ("test2", 1000),
            ("test3", 10000),
            ("test4", 999999),
        ]

        for test_str, mod_val in test_cases:
            result = hash_str2int(test_str, mod=mod_val)
            assert 0 <= result < mod_val

    def test_hexdigest_conversion(self):
        """Test the hexdigest to integer conversion"""
        test_string = "hello"
        hash_obj = hashlib.sha1(test_string.encode("utf-8"))
        hex_digest = hash_obj.hexdigest()
        expected_int = int(hex_digest, 16) % (10**8)

        result = hash_str2int(test_string)
        assert result == expected_int

    def test_consistent_with_direct_calculation(self):
        """Test that function matches direct hashlib usage"""
        test_strings = ["a", "b", "abc", "hello world", "12345"]

        for test_str in test_strings:
            direct_result = int(hashlib.sha1(test_str.encode("utf-8")).hexdigest(), 16) % (10**8)
            function_result = hash_str2int(test_str)
            assert function_result == direct_result

    def test_numeric_strings(self):
        """Test hashing numeric strings"""
        test_strings = ["123", "0", "999999", "3.14159", "-42"]

        for test_str in test_strings:
            result = hash_str2int(test_str)
            assert isinstance(result, int)
            assert 0 <= result < 10**8

    def test_whitespace_strings(self):
        """Test hashing strings with various whitespace"""
        test_strings = ["  leading", "trailing  ", "  both  ", "\ttab", "new\nline", "\r\nwindows"]

        for test_str in test_strings:
            result = hash_str2int(test_str)
            assert isinstance(result, int)
            assert 0 <= result < 10**8


class TestConvertBytes:
    """Test suite for convert_bytes function"""

    def test_zero_bytes(self):
        """Test that 0 bytes returns '0 B'"""
        assert convert_bytes(0) == "0 B"

    def test_single_byte(self):
        """Test single byte values"""
        assert convert_bytes(1) == "1 B"
        assert convert_bytes(999) == "999 B"

    def test_kilobyte_range(self):
        """Test values in kilobyte range with different precisions"""
        # Exactly 1 KB
        assert convert_bytes(1024) == "1.00 KB"

        # Values that should show 1 decimal place (10-99.9 range)
        assert convert_bytes(15360) == "15.0 KB"  # 15 KB exactly
        assert convert_bytes(10752) == "10.5 KB"  # 10.5 KB

        # Values that should show 2 decimal places (1-9.99 range)
        assert convert_bytes(2048) == "2.00 KB"  # 2 KB exactly
        assert convert_bytes(3072) == "3.00 KB"  # 3 KB exactly
        assert convert_bytes(5120) == "5.00 KB"  # 5 KB exactly

    def test_megabyte_range(self):
        """Test values in megabyte range"""
        # Exactly 1 MB
        assert convert_bytes(1048576) == "1.00 MB"

        # Values with different precision requirements
        assert convert_bytes(15728640) == "15.0 MB"  # 15.0 MB
        assert convert_bytes(11010048) == "10.5 MB"  # 10.5 MB

    def test_gigabyte_range(self):
        """Test values in gigabyte range"""
        # Exactly 1 GB
        assert convert_bytes(1073741824) == "1.00 GB"

        # Large value that should show 0 decimal places
        assert convert_bytes(3221225472) == "3.00 GB"  # 3 GB exactly

    def test_terabyte_range(self):
        """Test values in terabyte range"""
        assert convert_bytes(1099511627776) == "1.00 TB"  # 1 TB

    def test_petabyte_range(self):
        """Test values in petabyte range"""
        assert convert_bytes(1125899906842624) == "1.00 PB"  # 1 PB

    def test_boundary_values(self):
        """Test values at unit boundaries"""
        # Just below 1 KB
        assert convert_bytes(1023) == "1023 B"

        # Just above 1 KB
        assert convert_bytes(1025) == "1.00 KB"

        # At 100 KB boundary (should switch to 0 decimal places)
        assert convert_bytes(102400) == "100 KB"
        assert convert_bytes(102300) == "99.9 KB"

    def test_precision_transitions(self):
        """Test the precision formatting transitions"""
        # Test transition from 2 decimal places to 1 decimal place
        assert convert_bytes(9216) == "9.00 KB"  # 9.00 KB (2 decimal places)
        assert convert_bytes(10240) == "10.0 KB"  # 10.0 KB (1 decimal place)

        # Test transition from 1 decimal place to 0 decimal places
        assert convert_bytes(102400) == "100 KB"  # 100 KB (0 decimal places)

    def test_large_values_no_overflow(self):
        """Test that very large values don't cause issues"""
        # Very large value that should use PB
        large_value = 10 * 1125899906842624  # 10 PB
        assert "PB" in convert_bytes(large_value)

        # Ensure we don't exceed available units
        huge_value = 100 * 1125899906842624  # 100 PB (still within PB range)
        assert "PB" in convert_bytes(huge_value)
