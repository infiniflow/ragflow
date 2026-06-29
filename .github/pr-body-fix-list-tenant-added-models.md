## Summary

<!-- Summarize the change in 1-2 sentences. -->
Fixes `ValueError: too many values to unpack (expected 3)` raised in `list_tenant_added_models` when a model's name contains `@` characters (e.g. LM Studio embedding model IDs with `@<quantization>` suffix such as `text-embedding-nomic-embed-text-v1.5@q8_0`).

The composite key `provider@instance@model_name` was being unpacked with `key.split("@")`, which raised `ValueError: too many values to unpack` whenever the model_name itself contained `@`. This crashed the entire call and left the tenant with **no** models in the response.

Replaced the unbounded split with `split("@", 2)` (max-split of 2 from the left) so the trailing `model_name` absorbs any extra `@` characters. `provider_id` and `instance_id` are UUIDs and never contain `@`, so the limit of 2 is safe. A defensive `try/except` logs and skips any future malformed key instead of raising.

Fixes #16467

## Checklist

- [x] I have tested these changes locally
- [x] I have added tests to cover my changes
- [ ] I have updated the documentation (if applicable)
- [x] My code follows the project's coding style and conventions

## Screenshots / Additional context

(If applicable, add screenshots or additional context here.)

Verification: `uv run pytest test/unit_test/api/apps/services/test_models_api_service_list_tenant_added_models.py -v` → 4 passed.
