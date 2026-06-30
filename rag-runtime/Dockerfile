FROM python:3.11-slim

WORKDIR /app
COPY pyproject.toml ./
COPY app ./app
RUN pip install --no-cache-dir .

ENV PY_RAG_RUNTIME_HOST=0.0.0.0
ENV PY_RAG_RUNTIME_PORT=8091

CMD ["sh", "-c", "uvicorn app.main:app --host ${PY_RAG_RUNTIME_HOST:-0.0.0.0} --port ${PY_RAG_RUNTIME_PORT:-8091}"]
