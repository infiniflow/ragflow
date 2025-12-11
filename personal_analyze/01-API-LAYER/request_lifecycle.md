# Request Lifecycle Analysis

## Tổng Quan

Mỗi HTTP request trong RAGFlow đi qua một pipeline xử lý với nhiều stages: middleware, authentication, validation, và business logic.

## Request Lifecycle Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      COMPLETE REQUEST LIFECYCLE                          │
└─────────────────────────────────────────────────────────────────────────┘

[1] CLIENT REQUEST
    │
    ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [2] NGINX REVERSE PROXY                                                │
│     ├── SSL termination (HTTPS → HTTP)                                │
│     ├── Request buffering                                             │
│     ├── Rate limiting (optional)                                      │
│     └── Forward to upstream: ragflow-server:9380                      │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [3] QUART ASGI SERVER                                                  │
│     ├── Parse HTTP request                                            │
│     ├── Create request context                                        │
│     └── Route to WSGI app                                             │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [4] CORS MIDDLEWARE                                                    │
│     ├── Check Origin header                                           │
│     ├── Add CORS headers to response                                  │
│     │   - Access-Control-Allow-Origin: *                              │
│     │   - Access-Control-Allow-Methods: GET, POST, PUT, DELETE        │
│     │   - Access-Control-Allow-Headers: *                             │
│     └── Handle OPTIONS preflight (return 200)                         │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [5] SESSION MIDDLEWARE                                                 │
│     ├── Load session from Redis (if cookie present)                   │
│     ├── Initialize g.session object                                   │
│     └── Session data available throughout request                     │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [6] AUTHENTICATION (@app.before_request)                               │
│     ├── _load_user() called                                           │
│     ├── Parse Authorization header                                    │
│     ├── Validate JWT or API token                                     │
│     ├── Query user from database                                      │
│     └── Set g.user (or None for anonymous)                            │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [7] BLUEPRINT ROUTER                                                   │
│     ├── Match URL pattern to blueprint                                │
│     │   /api/v1/kb/* → kb_app                                        │
│     │   /api/v1/document/* → document_app                            │
│     │   /v1/conversation/* → conversation_app                        │
│     ├── Extract URL parameters                                        │
│     └── Call route handler                                            │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [8] ROUTE DECORATORS                                                   │
│     │                                                                 │
│     ├── @login_required                                               │
│     │   └── Check if g.user is set, else return 401                   │
│     │                                                                 │
│     ├── @validate_request("param1", "param2")                         │
│     │   └── Check required params exist in request body               │
│     │                                                                 │
│     └── Custom decorators (@rate_limit, @cache, etc.)                 │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [9] ROUTE HANDLER (async function)                                     │
│     │                                                                 │
│     ├── Parse request body                                            │
│     │   req = await request.json                                      │
│     │   form = await request.form                                     │
│     │   files = await request.files                                   │
│     │                                                                 │
│     ├── Authorization checks                                          │
│     │   check_kb_team_permission(kb, user.id)                         │
│     │                                                                 │
│     ├── Call Service Layer                                            │
│     │   result = ServiceClass.method(params)                          │
│     │                                                                 │
│     └── Format response                                               │
│         return get_json_result(data=result)                           │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [10] SERVICE LAYER                                                     │
│     │                                                                 │
│     ├── Business logic execution                                      │
│     ├── Database operations (Peewee ORM)                              │
│     ├── External service calls (LLM, storage)                         │
│     └── Return processed data                                         │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [11] RESPONSE FORMATTING                                               │
│     │                                                                 │
│     ├── get_json_result(code, message, data)                          │
│     │   {                                                             │
│     │     "code": 0,                                                  │
│     │     "message": "success",                                       │
│     │     "data": {...}                                               │
│     │   }                                                             │
│     │                                                                 │
│     └── Custom JSON encoder for special types                         │
│         - datetime → ISO string                                       │
│         - Decimal → float                                             │
│         - Model → dict                                                │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [12] ERROR HANDLING (@app.errorhandler)                                │
│     │                                                                 │
│     ├── Catch unhandled exceptions                                    │
│     ├── Log error with traceback                                      │
│     ├── Map exception to HTTP status code                             │
│     │   - Unauthorized → 401                                          │
│     │   - LookupError → 404                                           │
│     │   - PermissionError → 403                                       │
│     │   - Exception → 500                                             │
│     └── Return error response                                         │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [13] RESPONSE SENT TO CLIENT                                           │
│     ├── HTTP status code                                              │
│     ├── Response headers                                              │
│     └── JSON body                                                     │
└───────────────────────────────────────────────────────────────────────┘
```

## Code Examples

### Middleware Stack

```python
# /api/apps/__init__.py

# 1. Create Quart app
app = Quart(__name__)

# 2. Apply CORS middleware
app = cors(app, allow_origin="*")

# 3. Configure strict slashes
app.url_map.strict_slashes = False

# 4. Custom JSON encoder
app.json_encoder = CustomJSONEncoder

# 5. Session configuration
app.config["SESSION_TYPE"] = "redis"
app.config["SESSION_REDIS"] = redis_connection
app.config["MAX_CONTENT_LENGTH"] = 1024 * 1024 * 1024  # 1GB

# 6. Global error handler
app.errorhandler(Exception)(server_error_response)

# 7. Before request hook (authentication)
@app.before_request
def before_request():
    _load_user()
```

### Request Validation Decorator

```python
def validate_request(*args, **kwargs):
    """
    Decorator to validate required request parameters.

    Usage:
        @validate_request("kb_id", "name")  # Required params
        @validate_request("status", status=["active", "inactive"])  # Enum validation
    """
    def process_args(input_arguments):
        no_arguments = []
        error_arguments = []

        # Check required args exist
        for arg in args:
            if arg not in input_arguments:
                no_arguments.append(arg)

        # Check enum values
        for k, v in kwargs.items():
            config_value = input_arguments.get(k, None)
            if config_value is None:
                no_arguments.append(k)
            elif isinstance(v, (tuple, list)):
                if config_value not in v:
                    error_arguments.append((k, set(v)))

        if no_arguments or error_arguments:
            error_string = f"Required arguments missing: {','.join(no_arguments)}"
            if error_arguments:
                error_string += f"; Invalid values: {error_arguments}"
            return error_string
        return None

    def wrapper(func):
        @wraps(func)
        async def decorated_function(*_args, **_kwargs):
            # Get request data
            body = await request.json or (await request.form).to_dict()

            # Validate
            errs = process_args(body)
            if errs:
                return get_json_result(
                    code=RetCode.ARGUMENT_ERROR,
                    message=errs
                )

            # Call handler
            if inspect.iscoroutinefunction(func):
                return await func(*_args, **_kwargs)
            return func(*_args, **_kwargs)

        return decorated_function
    return wrapper
```

### Response Formatting

```python
def get_json_result(
    code: RetCode = RetCode.SUCCESS,
    message: str = "success",
    data: Any = None
) -> Response:
    """
    Standard JSON response formatter.

    Args:
        code: Return code (0 = success)
        message: Human-readable message
        data: Response payload

    Returns:
        Flask Response with JSON body
    """
    response = {
        "code": code,
        "message": message,
        "data": data
    }
    return jsonify(response)


class CustomJSONEncoder(json.JSONEncoder):
    """Custom JSON encoder for special types."""

    def default(self, obj):
        if isinstance(obj, datetime):
            return obj.isoformat()
        if isinstance(obj, Decimal):
            return float(obj)
        if hasattr(obj, 'to_dict'):
            return obj.to_dict()
        if isinstance(obj, bytes):
            return base64.b64encode(obj).decode()
        return super().default(obj)
```

### Error Handler

```python
def server_error_response(e):
    """
    Global exception handler for unhandled errors.
    """
    logging.error(
        "Unhandled exception",
        exc_info=(type(e), e, e.__traceback__)
    )

    msg = repr(e).lower()

    # Map exception types to HTTP codes
    if getattr(e, "code", None) == 401 or "unauthorized" in msg:
        return get_json_result(
            code=RetCode.UNAUTHORIZED,
            message=repr(e)
        ), 401

    if isinstance(e, LookupError) or "not found" in msg:
        return get_json_result(
            code=RetCode.DATA_ERROR,
            message=repr(e)
        ), 404

    if isinstance(e, PermissionError) or "permission" in msg:
        return get_json_result(
            code=RetCode.FORBIDDEN,
            message=repr(e)
        ), 403

    # Document store specific errors
    if "index_not_found_exception" in repr(e):
        return get_json_result(
            code=RetCode.EXCEPTION_ERROR,
            message="No chunk found. Please upload and parse files first."
        )

    # Generic server error
    return get_json_result(
        code=RetCode.EXCEPTION_ERROR,
        message=repr(e)
    ), 500
```

## Typical Request Example

### Request

```http
POST /api/v1/kb/create HTTP/1.1
Host: localhost:9380
Authorization: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
    "name": "My Knowledge Base",
    "parser_id": "pdf"
}
```

### Processing Timeline

```
T+0ms   : Request received by Nginx
T+1ms   : Forwarded to Quart server
T+2ms   : CORS headers added
T+3ms   : Session loaded from Redis
T+5ms   : _load_user() validates JWT
T+10ms  : User queried from MySQL
T+12ms  : Blueprint router matches /api/v1/kb/create
T+13ms  : @login_required passes (user exists)
T+14ms  : @validate_request("name") validates params
T+15ms  : Handler async function called
T+20ms  : KnowledgebaseService.create() called
T+25ms  : KB inserted into MySQL
T+28ms  : ES index created
T+30ms  : Response formatted
T+31ms  : JSON response sent
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
Access-Control-Allow-Origin: *

{
    "code": 0,
    "message": "success",
    "data": {
        "id": "kb_123abc",
        "name": "My Knowledge Base",
        "parser_id": "pdf",
        "created_at": "2024-01-15T10:30:00Z"
    }
}
```

## Performance Considerations

### Connection Pooling

```python
# Database connection pool
db = PooledMySQLDatabase(
    database,
    max_connections=32,
    stale_timeout=300,
    **connection_params
)

# Redis connection pool
redis_pool = redis.ConnectionPool(
    host=redis_host,
    port=redis_port,
    max_connections=100
)
```

### Async I/O

```python
# All route handlers are async
@manager.route("/endpoint", methods=["POST"])
async def handler():
    # Async request parsing
    req = await request.json

    # Async file handling
    files = await request.files

    # Run blocking I/O in thread pool
    result = await asyncio.to_thread(blocking_operation)

    return get_json_result(data=result)
```

### Response Streaming

```python
# For large responses, use streaming
def stream():
    for chunk in generate_chunks():
        yield chunk

resp = Response(stream(), mimetype="text/event-stream")
resp.headers.add_header("X-Accel-Buffering", "no")
return resp
```

## Logging

```python
# Request logging
@app.before_request
def log_request():
    logging.info(f"{request.method} {request.path}")

# Response logging
@app.after_request
def log_response(response):
    logging.info(f"Response: {response.status_code}")
    return response
```

## Related Files

- `/api/apps/__init__.py` - App initialization
- `/api/ragflow_server.py` - Server entry point
- `/api/utils/api_utils.py` - API utilities
- `/api/validation.py` - Request validation
