import logging
from datetime import UTC, datetime
from typing import Annotated

from fastapi import APIRouter, Depends, status
from fastapi.responses import JSONResponse
from redis.asyncio import Redis

from ...core.config import settings
from ...core.health import check_redis_health
from ...core.utils.cache import async_get_redis
from ...schemas.health import HealthCheckResponse, ReadyCheckResponse

router = APIRouter(tags=["health"])

STATUS_HEALTHY = "healthy"
STATUS_UNHEALTHY = "unhealthy"

# Probes must always hit the app, never a browser/proxy cache.
NO_STORE_HEADERS = {"Cache-Control": "no-store"}

LOGGER = logging.getLogger(__name__)


@router.get("/health", response_model=HealthCheckResponse)
async def health() -> JSONResponse:
    """Liveness: the process is up and serving requests."""
    response = HealthCheckResponse(
        status=STATUS_HEALTHY,
        environment=settings.ENVIRONMENT.value,
        version=settings.APP_VERSION,
        timestamp=datetime.now(UTC).isoformat(timespec="seconds"),
    )
    return JSONResponse(status_code=status.HTTP_200_OK, content=response.model_dump(), headers=NO_STORE_HEADERS)


@router.get(
    "/ready",
    response_model=ReadyCheckResponse,
    responses={status.HTTP_503_SERVICE_UNAVAILABLE: {"model": ReadyCheckResponse}},
)
async def ready(redis: Annotated[Redis, Depends(async_get_redis)]) -> JSONResponse:
    """Readiness: Redis (sessions, cache, rate limiting) is reachable."""
    redis_status = await check_redis_health(redis=redis)
    LOGGER.debug(f"Redis health check status: {redis_status}")

    overall_status = STATUS_HEALTHY if redis_status else STATUS_UNHEALTHY
    http_status = status.HTTP_200_OK if redis_status else status.HTTP_503_SERVICE_UNAVAILABLE

    response = ReadyCheckResponse(
        status=overall_status,
        environment=settings.ENVIRONMENT.value,
        version=settings.APP_VERSION,
        app=STATUS_HEALTHY,
        redis=overall_status,
        timestamp=datetime.now(UTC).isoformat(timespec="seconds"),
    )
    return JSONResponse(status_code=http_status, content=response.model_dump(), headers=NO_STORE_HEADERS)
