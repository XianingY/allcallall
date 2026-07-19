FROM python:3.12-slim

RUN groupadd --gid 10001 runner && useradd --uid 10001 --gid runner --no-create-home runner
WORKDIR /app
COPY pyproject.toml /tmp/runner-deps/pyproject.toml
RUN mkdir -p /tmp/runner-deps/app \
    && touch /tmp/runner-deps/app/__init__.py \
    && pip install --no-cache-dir /tmp/runner-deps \
    && rm -rf /tmp/runner-deps

COPY pyproject.toml ./
COPY app ./app
RUN pip install --no-cache-dir --no-deps .

USER 10001:10001
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
EXPOSE 8093
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8093", "--no-access-log"]
