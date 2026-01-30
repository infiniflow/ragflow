# URL Validation Fix for Paperless-ngx Connector

## Problem

User encountered connection error:
```
2026-01-30 23:49:34,336 ERROR    46 Error listing documents (page 1): 
Request to Paperless-ngx API failed: Invalid URL 'http:192.168.1.6:8000/api/documents/': No host supplied
```

### Root Cause

The user entered `http:192.168.1.6:8000` instead of `http://192.168.1.6:8000` (missing the double slashes `//`).

The connector's `__init__` method only stripped trailing slashes but didn't validate or normalize the URL, causing Python's `urlparse` to fail when constructing API URLs.

## Solution Implemented

Added automatic URL normalization in the `PaperlessNgxConnector.__init__()` method via a new `_normalize_url()` function.

### URL Normalization Features

1. **Fix Missing `//` After Scheme**
   ```python
   http:192.168.1.6:8000  →  http://192.168.1.6:8000
   https:paperless.com    →  https://paperless.example.com
   ```

2. **Add Missing Scheme (defaults to HTTPS)**
   ```python
   paperless.example.com  →  https://paperless.example.com
   192.168.1.6:8000      →  https://192.168.1.6:8000
   ```

3. **Remove Trailing Slashes**
   ```python
   https://paperless.com/  →  https://paperless.example.com
   ```

4. **Validate URL Structure**
   - Empty URLs raise `ConnectorValidationError`
   - Malformed URLs raise informative error messages

## Code Changes

### Before
```python
def __init__(self, base_url: str, ...):
    self.base_url = base_url.rstrip("/")  # Only strips trailing slashes
    # ...
```

### After
```python
def __init__(self, base_url: str, ...):
    self.base_url = self._normalize_url(base_url)  # Normalizes and validates
    # ...

def _normalize_url(self, url: str) -> str:
    # 1. Validate not empty
    # 2. Fix missing // after scheme
    # 3. Add default https:// if no scheme
    # 4. Validate URL structure
    # 5. Remove trailing slashes
```

## Test Coverage

Added 6 new test cases:

1. `test_url_normalization_missing_double_slash()` - Fixes `http:hostname`
2. `test_url_normalization_no_scheme()` - Adds `https://` prefix
3. `test_url_normalization_trailing_slash()` - Removes trailing slashes
4. `test_url_normalization_already_valid()` - Preserves valid URLs
5. `test_url_validation_empty_url()` - Rejects empty URLs
6. `test_url_validation_invalid_url()` - Rejects malformed URLs

All tests pass ✅

## User Experience

### Before Fix
```
❌ Error: Invalid URL 'http:192.168.1.6:8000/api/documents/': No host supplied
```

User had to:
- Figure out the URL was malformed
- Find and fix the missing `//`
- Retry connection

### After Fix
```
✅ URL automatically corrected: http:192.168.1.6:8000 → http://192.168.1.6:8000
✅ Connection successful
```

User can enter URLs in various formats:
- `http:192.168.1.6:8000` (missing `//`)
- `https:paperless.com` (missing `//`)
- `paperless.example.com` (missing scheme)
- `http://localhost:8000` (already valid)

All will work correctly!

## Documentation Updates

Updated the following documentation:

1. **`docs/paperless_ngx_connector.md`**
   - Added URL normalization examples
   - Explained automatic fixes
   - Recommended including scheme for clarity

2. **`GETTING_STARTED_PAPERLESS.md`**
   - Added URL format guidance
   - Explained auto-correction features
   - Provided clear examples

## Files Changed

- ✅ `common/data_source/paperless_ngx_connector.py` - URL normalization logic
- ✅ `test/unit/test_paperless_ngx_connector.py` - 6 new test cases
- ✅ `docs/paperless_ngx_connector.md` - Updated documentation
- ✅ `GETTING_STARTED_PAPERLESS.md` - Updated getting started guide

## Backward Compatibility

✅ **Fully backward compatible**

- Valid URLs remain unchanged
- New behavior only affects malformed URLs
- No breaking changes to API

## Deployment

This fix is included in commit `ed267b4`. Users who rebuild their Docker image will automatically get the fix.

No configuration changes required - the fix is transparent to users.

---

**Status: Fixed and Tested ✅**
