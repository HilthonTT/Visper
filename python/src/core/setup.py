from collections.abc import AsyncGenerator, Callable
from contextlib import _AsyncGeneratorContextManager, asynccontextmanager
import logging
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

from ..api.v1.ai_bot import get_enhancement_service
from ..core.utils.rate_limit import rate_limiter
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

logger = logging.getLogger(__name__)

async def create_redis_cache_pool() -> None:
    cache.pool = redis.ConnectionPool.from_url(settings.REDIS_CACHE_URL)
    cache.client = redis.Redis.from_pool(cache.pool)
    
async def close_redis_cache_pool() -> None:
    if cache.client is not None:
        await cache.client.aclose()
        
async def create_redis_queue_pool() -> None:
    queue.pool = await create_pool(
        RedisSettings(
            host=settings.REDIS_QUEUE_HOST, 
            port=settings.REDIS_QUEUE_PORT,
            password=settings.REDIS_QUEUE_PASSWORD
        )
    )
    
async def close_redis_queue_pool() -> None:
    if queue.pool is not None:
        await queue.pool.aclose()
        
async def create_redis_rate_limit_pool() -> None:
    rate_limiter.initialize(settings.REDIS_RATE_LIMIT_URL)
    
async def close_redis_rate_limit_pool() -> None:
    if rate_limiter.client is not None:
        await rate_limiter.client.aclose()
        
async def set_threadpool_tokens(number_of_tokens: int = 100) -> None:
    limiter = anyio.to_thread.current_default_thread_limiter()
    limiter.total_tokens = number_of_tokens

async def warmup_ollama():
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
    )
) -> Callable[[FastAPI], _AsyncGeneratorContextManager[Any]]:
    """Factory to create a lifespan async context manager for a FastAPI app."""
    
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncGenerator:
        from asyncio import Event
        
        initialization_complete = Event()
        app.state.initialization_complete = initialization_complete
        
        await set_threadpool_tokens()
        
        try:
            if isinstance(settings, RedisCacheSettings):
                await create_redis_cache_pool()

            if isinstance(settings, RedisQueueSettings):
                await create_redis_queue_pool()

            if isinstance(settings, RedisRateLimiterSettings):
                await create_redis_rate_limit_pool()

            initialization_complete.set()
            
            await warmup_ollama()

            yield
        finally:
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

    This function initializes a FastAPI application and configures it with various settings
    and handlers based on the type of the `settings` object provided.

    Parameters
    ----------
    router : APIRouter
        The APIRouter object containing the routes to be included in the FastAPI application.

    settings
        An instance representing the settings for configuring the FastAPI application.
        It determines the configuration applied:

        - AppSettings: Configures basic app metadata like name, description, contact, and license info.
        - DatabaseSettings: Adds event handlers for initializing database tables during startup.
        - RedisCacheSettings: Sets up event handlers for creating and closing a Redis cache pool.
        - ClientSideCacheSettings: Integrates middleware for client-side caching.
        - CORSSettings: Integrates CORS middleware with specified origins.
        - RedisQueueSettings: Sets up event handlers for creating and closing a Redis queue pool.
        - RedisRateLimiterSettings: Sets up event handlers for creating and closing a Redis rate limiter pool.
        - EnvironmentSettings: Conditionally sets documentation URLs and integrates custom routes for API documentation
          based on the environment type.

    create_tables_on_start : bool
        A flag to indicate whether to create database tables on application startup.
        Defaults to True.

    **kwargs
        Additional keyword arguments passed directly to the FastAPI constructor.

    Returns
    -------
    FastAPI
        A fully configured FastAPI application instance.

    The function configures the FastAPI application with different features and behaviors
    based on the provided settings. It includes setting up database connections, Redis pools
    for caching, queue, and rate limiting, client-side caching, and customizing the API documentation
    based on the environment settings.
    """
    # --- before creating application ---
    if isinstance(settings, AppSettings):
        to_update = {
            "title": settings.APP_NAME,
            "description": settings.APP_DESCRIPTION,
            "license_info": {"name": settings.LICENSE_NAME},
        } 
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
        application.add_middleware(
            CORSMiddleware,
            allow_origins=settings.CORS_ORIGINS,
            allow_credentials=True,
            allow_methods=settings.CORS_METHODS,
            allow_headers=settings.CORS_HEADERS,
        )
    application.add_middleware(LoggerMiddleware)
    
    if isinstance(settings, EnvironmentSettings):
        if settings.ENVIRONMENT != EnvironmentOption.PRODUCTION:
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
