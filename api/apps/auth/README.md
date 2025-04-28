# Auth

The Auth module provides implementations of OAuth2 and OpenID Connect (OIDC) authentication for integration with third-party identity providers. 

**Features**

- Supports both OAuth2 and OIDC authentication protocols
- Automatic OIDC configuration discovery (via `/.well-known/openid-configuration`)
- JWT token validation
- Unified user information handling

## Usage

```python
# OAuth2 configuration
oauth_config = {
    "type": "oauth2",
    "client_id": "your_client_id",
    "client_secret": "your_client_secret",
    "authorization_url": "https://provider.com/oauth/authorize",
    "token_url": "https://provider.com/oauth/token",
    "userinfo_url": "https://provider.com/oauth/userinfo",
    "redirect_uri": "https://your-app.com/oauth/callback/<channel>"
}

# OIDC configuration
oidc_config = {
    "type": "oidc",
    "issuer": "https://provider.com/v1/oidc",
    "client_id": "your_client_id",
    "client_secret": "your_client_secret",
    "redirect_uri": "https://your-app.com/oauth/callback/<channel>"
}

# Get client instance
client = get_auth_client(oauth_config)  # or oidc_config
```

### Authentication Flow

1. Get authorization URL:
```python
auth_url = client.get_authorization_url()
```

2. After user authorization, exchange authorization code for token:
```python
token_response = client.exchange_code_for_token(authorization_code)
access_token = token_response["access_token"]
```

3. Fetch user information:
```python
user_info = client.fetch_user_info(access_token)
```

## User Information Structure

All authentication methods return user information following this structure:

```python
{
    "email": "user@example.com",
    "username": "username",
    "nickname": "User Name",
    "avatar_url": "https://example.com/avatar.jpg"
}
```
