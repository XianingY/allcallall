FROM python:3.12-slim

WORKDIR /app

ENV PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

COPY shared/pyproject.toml \
     shared/__init__.py \
     shared/models.py \
     shared/scoring.py \
     shared/utils.py \
     shared/py.typed \
     /opt/allcallall-shared/
RUN pip install /opt/allcallall-shared

COPY agent-runtime/pyproject.toml ./
COPY agent-runtime/app ./app
RUN pip install . \
    && groupadd --gid 65532 allcallall \
    && useradd --uid 65532 --gid allcallall --no-create-home allcallall

ENV PY_AGENT_RUNTIME_HOST=0.0.0.0
ENV PY_AGENT_RUNTIME_PORT=8090

USER 65532:65532
EXPOSE 8090

CMD ["sh", "-c", "uvicorn app.main:app --host ${PY_AGENT_RUNTIME_HOST:-0.0.0.0} --port ${PY_AGENT_RUNTIME_PORT:-8090}"]
