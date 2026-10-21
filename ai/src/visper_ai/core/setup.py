import asyncio
import contextlib
import logging
from collections.abc import AsyncGenerator, Callable
from contextlib import _AsyncGeneratorContextManager, asynccontextmanager
from typing import Any

import anyio
import fastapi
import redis.asyncio as redis
from arq import create_pool
from arq.connections import RedisSettings
from fastapi import APIRouter, FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.openapi.docs import get_redoc_html, get_swagger_ui_html
from fastapi.openapi.utils import get_openapi

from ..api.v1.ai_bot import close_enhancement_service, get_enhancement_service
from ..middleware.client_cache_middleware import ClientCacheMiddleware
from ..middleware.logger_middleware import LoggerMiddleware
from .config import (
    AppSettings,
    ClientSideCacheSettings,
    CORSSettings,
    EnvironmentOption,
    EnvironmentSettings,
    RedisCacheSettings,
    RedisQueueSettings,
    RedisRateLimiterSettings,
    settings,
)
from .utils import cache, queue
from .utils.rate_limit import rate_limiter

logger = logging.getLogger(__name__)


async def create_redis_cache_pool() -> None:
    cache.pool = redis.ConnectionPool.from_url(settings.REDIS_CACHE_URL)
    # from_pool() makes the client own the pool, so client.aclose() also closes the pool.
    cache.client = redis.Redis.from_pool(cache.pool)


async def close_redis_cache_pool() -> None:
    if cache.client is not None:
        await cache.client.aclose()
    cache.client = None
    cache.pool = None


async def create_redis_queue_pool() -> None:
    queue.pool = await create_pool(
        RedisSettings(
            host=settings.REDIS_QUEUE_HOST,
            port=settings.REDIS_QUEUE_PORT,
            password=settings.REDIS_QUEUE_PASSWORD,
        )
    )


async def close_redis_queue_pool() -> None:
    if queue.pool is not None:
        await queue.pool.aclose()
    queue.pool = None


async def create_redis_rate_limit_pool() -> None:
    rate_limiter.initialize(settings.REDIS_RATE_LIMIT_URL)


async def close_redis_rate_limit_pool() -> None:
    await rate_limiter.close()


async def set_threadpool_tokens(number_of_tokens: int = 100) -> None:
    limiter = anyio.to_thread.current_default_thread_limiter()
    limiter.total_tokens = number_of_tokens


async def warmup_ollama() -> None:
    try:
        service = get_enhancement_service()
        logger.info("Warming up Ollama model...")
        await service.ollama.generate(
            prompt="Hello",
            max_tokens=5,
        )
        logger.info("Ollama warmup complete")
    except Exception as e:
        logger.warning(f"Ollama warmup failed (will retry on first request): {e}")


def lifespan_factory(
    settings: (
        RedisCacheSettings
        | AppSettings
        | ClientSideCacheSettings
        | CORSSettings
        | RedisQueueSettings
        | RedisRateLimiterSettings
        | EnvironmentSettings
    ),
) -> Callable[[FastAPI], _AsyncGeneratorContextManager[Any]]:
    """Factory to create a lifespan async context manager for a FastAPI app."""

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncGenerator:
        initialization_complete = asyncio.Event()
        app.state.initialization_complete = initialization_complete

        await set_threadpool_tokens()

        warmup_task: asyncio.Task | None = None
        try:
            if isinstance(settings, RedisCacheSettings):
                await create_redis_cache_pool()

            if isinstance(settings, RedisQueueSettings):
                await create_redis_queue_pool()

            if isinstance(settings, RedisRateLimiterSettings):
                await create_redis_rate_limit_pool()

            initialization_complete.set()

            # Warm up in the background: awaiting it blocked startup (and /health)
            # for up to OLLAMA_TIMEOUT while a cold model was loading.
            warmup_task = asyncio.create_task(warmup_ollama())

            yield
        finally:
            if warmup_task is not None and not warmup_task.done():
                warmup_task.cancel()
                with contextlib.suppress(asyncio.CancelledError):
                    await warmup_task

            await close_enhancement_service()

            if isinstance(settings, RedisCacheSettings):
                await close_redis_cache_pool()

            if isinstance(settings, RedisQueueSettings):
                await close_redis_queue_pool()

            if isinstance(settings, RedisRateLimiterSettings):
                await close_redis_rate_limit_pool()

    return lifespan


def create_application(
    router: APIRouter,
    settings: (
        RedisCacheSettings
        | AppSettings
        | ClientSideCacheSettings
        | CORSSettings
        | RedisQueueSettings
        | RedisRateLimiterSettings
        | EnvironmentSettings
    ),
    lifespan: Callable[[FastAPI], _AsyncGeneratorContextManager[Any]] | None = None,
    **kwargs: Any,
) -> FastAPI:
    """Creates and configures a FastAPI application based on the provided settings.

    The type of the ``settings`` object decides which features are enabled:

    - AppSettings: basic app metadata (name, description, version, license).
    - RedisCacheSettings / RedisQueueSettings / RedisRateLimiterSettings: Redis pools
      created and closed in the lifespan.
    - ClientSideCacheSettings: client-side caching middleware.
    - CORSSettings: CORS middleware.
    - EnvironmentSettings: API docs exposed only outside production.

    ``lifespan`` overrides the default lifespan (useful in tests); extra ``kwargs`` are
    passed to the FastAPI constructor.
    """
    # --- before creating application ---
    if isinstance(settings, AppSettings):
        to_update: dict[str, Any] = {
            "title": settings.APP_NAME,
            "description": settings.APP_DESCRIPTION or "",
            "version": settings.APP_VERSION,
        }
        # FastAPI's OpenAPI License model requires a name; {"name": None} broke /openapi.json.
        if settings.LICENSE_NAME:
            to_update["license_info"] = {"name": settings.LICENSE_NAME}
        kwargs.update(to_update)

    if isinstance(settings, EnvironmentSettings):
        kwargs.update({"docs_url": None, "redoc_url": None, "openapi_url": None})

    # Use custom lifespan if provided, otherwise use default factory
    if lifespan is None:
        lifespan = lifespan_factory(settings)

    application = FastAPI(lifespan=lifespan, **kwargs)
    application.include_router(router)

    if isinstance(settings, ClientSideCacheSettings):
        application.add_middleware(ClientCacheMiddleware, max_age=settings.CLIENT_CACHE_MAX_AGE)

    if isinstance(settings, CORSSettings):
        # Browsers reject `Access-Control-Allow-Origin: *` with credentials, and Starlette
        # then reflects any Origin instead — i.e. credentialed requests from every site.
        allow_credentials = "*" not in settings.CORS_ORIGINS
        application.add_middleware(
            CORSMiddleware,
            allow_origins=settings.CORS_ORIGINS,
            allow_credentials=allow_credentials,
            allow_methods=settings.CORS_METHODS,
            allow_headers=settings.CORS_HEADERS,
        )
    application.add_middleware(LoggerMiddleware)

    if isinstance(settings, EnvironmentSettings) and settings.ENVIRONMENT != EnvironmentOption.PRODUCTION:
        docs_router = APIRouter()

        @docs_router.get("/docs", include_in_schema=False)
        async def get_swagger_documentation() -> fastapi.responses.HTMLResponse:
            return get_swagger_ui_html(openapi_url="/openapi.json", title="docs")

        @docs_router.get("/redoc", include_in_schema=False)
        async def get_redoc_documentation() -> fastapi.responses.HTMLResponse:
            return get_redoc_html(openapi_url="/openapi.json", title="docs")

        @docs_router.get("/openapi.json", include_in_schema=False)
        async def openapi() -> dict[str, Any]:
            out: dict = get_openapi(title=application.title, version=application.version, routes=application.routes)
            return out

        application.include_router(docs_router)
    return application
