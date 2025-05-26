# Gunicorn configuration file for RAGFlow production deployment
import multiprocessing
import os

# Server socket
bind = f"{os.environ.get('RAGFLOW_HOST_IP', '0.0.0.0')}:{os.environ.get('RAGFLOW_HOST_PORT', '9380')}"
backlog = 2048

# Worker processes
workers = int(os.environ.get('GUNICORN_WORKERS', min(multiprocessing.cpu_count() * 2 + 1, 8)))
worker_class = 'sync'
worker_connections = 1000
timeout = 120
keepalive = 2
max_requests = 1000
max_requests_jitter = 100

# Restart workers after this many requests, to help prevent memory leaks
preload_app = True

# Logging
accesslog = '-'
errorlog = '-'
loglevel = 'info'
access_log_format = '%(h)s %(l)s %(u)s %(t)s "%(r)s" %(s)s %(b)s "%(f)s" "%(a)s" %(D)s'

# Process naming
proc_name = 'ragflow_gunicorn'

# Server mechanics
daemon = False
pidfile = '/tmp/ragflow_gunicorn.pid'
tmp_upload_dir = None

# Security
limit_request_line = 8192
limit_request_fields = 200
limit_request_field_size = 8190

# Performance tuning for RAGFlow
worker_tmp_dir = '/dev/shm'  # Use memory for temporary files if available

# SSL (if needed)
# keyfile = None
# certfile = None

# Environment variables that gunicorn should pass to workers
raw_env = [
    'PYTHONPATH=/ragflow/',
]

def when_ready(server):
    """Called just after the server is started."""
    server.log.info("RAGFlow Gunicorn server is ready. Production mode active.")

def worker_int(worker):
    """Called just after a worker exited on SIGINT or SIGQUIT."""
    worker.log.info("RAGFlow worker received INT or QUIT signal")

def pre_fork(server, worker):
    """Called just before a worker is forked."""
    server.log.info("RAGFlow worker about to be forked")

def post_fork(server, worker):
    """Called just after a worker has been forked."""
    server.log.info("RAGFlow worker spawned (pid: %s)", worker.pid)

def worker_abort(worker):
    """Called when a worker received the SIGABRT signal."""
    worker.log.info("RAGFlow worker received SIGABRT signal")