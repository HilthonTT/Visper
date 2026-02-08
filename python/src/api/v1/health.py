import logging
from datetime import UTC, datetime
from typing import Annotated
from fastapi import APIRouter, Depends, status
from fastapi.responses import JSONResponse
from redis.asyncio import Redis

from ...core.health import check_redis_health
from ...core.utils.cache import async_get_redis
from ...schemas.health import HealthCheckResponse, ReadyCheckResponse
from ...core.config import settings

router = APIRouter(tags=["health"])

STATUS_HEALTHY = "healthy"
STATUS_UNHEALTHY = "unhealthy"

LOGGER = logging.getLogger(__name__)

@router.get("/health", response_model=HealthCheckResponse)
async def health() -> JSONResponse:
    http_status = status.HTTP_200_OK
    response = {
        "status": STATUS_HEALTHY,
        "environment": settings.ENVIRONMENT.value,
        "version": settings.APP_VERSION,
        "timestamp": datetime.now(UTC).isoformat(timespec="seconds"),
    }
    return JSONResponse(status_code=http_status, content=response)
    
@router.get("/ready", response_model=ReadyCheckResponse)
async def ready(redis: Annotated[Redis, Depends(async_get_redis)]) -> JSONResponse:
    redis_status = await check_redis_health(redis=redis)
    LOGGER.debug(f"Redis health check status: {redis_status}")
    
    overall_status = STATUS_HEALTHY if redis_status else STATUS_UNHEALTHY
    http_status = status.HTTP_200_OK if overall_status == STATUS_HEALTHY else status.HTTP_503_SERVICE_UNAVAILABLE
    
    response = {
        "status": overall_status,
        "environment": settings.ENVIRONMENT.value,
        "version": settings.APP_VERSION,
        "app": STATUS_HEALTHY,
        "redis": STATUS_HEALTHY if redis_status else STATUS_UNHEALTHY,
        "timestamp": datetime.now(UTC).isoformat(timespec="seconds"),
    }

    return JSONResponse(status_code=http_status, content=response)
