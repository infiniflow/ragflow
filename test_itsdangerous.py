#!/usr/bin/env python3
"""Test itsdangerous token generation and parsing"""

from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer

# Also try from environment
# env_secret = 'db27abc74582358e85ad94a61a5e56e07fa260cd73894abd1eb738cb6d8b3146'
env_secret = '55d169ff069ae8a4c5f35bb0dee2397ae156a37db56ff4b36df3d0c954025faa'

# access_token '2d46e59a15dd11f1937338a74640adcc'
# user_token 'IjMyYTU0YjhhMTRiZTExZjE4ZjMzMzhhNzQ2NDBhZGNjIg.aaMQbw.5i9p6sTWDhVfzJDoRu3Eo1MP2nQ'
user_token = 'IjJkNDZlNTlhMTVkZDExZjE5MzczMzhhNzQ2NDBhZGNjIg.aaTyCQ.bhMImMrzn6HDO5aB3J_SOB8ZnSM'

print("=== Testing User's Token with different secret keys ===")
jwt = Serializer(secret_key=env_secret)
try:
    parsed = jwt.loads(user_token)
    if parsed == '2d46e59a15dd11f1937338a74640adcc':
        print(f"SUCCESS with secret_key='{env_secret}': {parsed}")
    else:
        print(f"FAILED with secret_key='{env_secret}': {parsed}")
except Exception as e:
    print(f"Failed with secret_key='{env_secret}': {e}")
