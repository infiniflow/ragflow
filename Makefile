install:
    poetry install
fmt:
    ruff format .
    ruff check --fix .
