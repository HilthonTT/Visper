# syntax=docker/dockerfile:1
FROM python:3.13-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    LOG_DIR=/app/logs \
    APP_HOST=0.0.0.0 \
    APP_PORT=8088

WORKDIR /app

RUN groupadd --system app && useradd --system --gid app --home-dir /app --no-create-home app

# Install the package and its dependencies from pyproject.toml.
COPY pyproject.toml README.md ./
COPY src ./src
RUN pip install . && mkdir -p /app/logs && chown -R app:app /app

USER app

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8088/api/v1/health', timeout=4).status == 200 else 1)"

# API (default). Worker: docker run <image> arq visper_ai.core.worker.settings.WorkerSettings
CMD ["sh", "-c", "exec uvicorn visper_ai.main:app --host \"$APP_HOST\" --port \"$APP_PORT\" --proxy-headers"]
