FROM python:3.12-slim

WORKDIR /app
COPY pyproject.toml ./
COPY app ./app
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --timeout 120 --retries 5 . || \
    (sleep 5 && pip install --timeout 120 --retries 5 .)

RUN useradd --uid 10001 --no-create-home --shell /usr/sbin/nologin interview-mcp \
    && mkdir -p /data /run/interview-tls /run/secrets \
    && chown -R 10001:10001 /app /data /run/interview-tls /run/secrets

USER 10001:10001
EXPOSE 8443
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8443", "--ssl-keyfile", "/run/interview-tls/interview-mcp.key", "--ssl-certfile", "/run/interview-tls/interview-mcp.crt"]
