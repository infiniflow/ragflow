FROM python:3.12-slim

# Install JRE — OpenDataLoader requires Java 11+.
# This keeps the JRE dependency isolated inside this container;
# the RAGFlow host does not need Java installed.
RUN apt-get update && apt-get install -y --no-install-recommends \
        default-jre-headless \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

EXPOSE 9383

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "9383"]
