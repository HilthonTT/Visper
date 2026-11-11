import logging
import uuid
from collections.abc import AsyncGenerator
from datetime import UTC, datetime
from typing import Optional

from redis.asyncio import ConnectionPool, Redis

from ...schemas.rate_limit import sanitize_path

logger = logging.getLogger(__name__)


class RateLimiter:
    _instance: Optional["RateLimiter"] = None
    pool: ConnectionPool | None = None
    client: Redis | None = None

    def __new__(cls) -> "RateLimiter":
        if cls._instance is None:
            cls._instance = super().__new__(cls)
        return cls._instance

    @classmethod
    def initialize(cls, redis_url: str) -> None:
        instance = cls()
        if instance.pool is None:
            instance.pool = ConnectionPool.from_url(redis_url)
            instance.client = Redis(connection_pool=instance.pool)

    @classmethod
    async def close(cls) -> None:
        """Close the client *and* its pool.

        ``Redis(connection_pool=...)`` does not own the pool, so ``client.aclose()``
        alone leaves every pooled connection open.
        """
        instance = cls()
        if instance.client is not None:
            await instance.client.aclose()
        if instance.pool is not None:
            await instance.pool.aclose()
        instance.client = None
        instance.pool = None

    @classmethod
    def get_client(cls) -> Redis:
        instance = cls()
        if instance.client is None:
            logger.error("Redis rate limit client is not initialized")
            raise RuntimeError("Redis rate limit client is not initialized")
        return instance.client

    async def is_rate_limited(self, user_id: str, path: str, limit: int, period: int) -> bool:
        """Fixed-window rate limit check (INCR + EXPIRE)."""
        client = self.get_client()
        current_timestamp = int(datetime.now(UTC).timestamp())
        window_start = current_timestamp - (current_timestamp % period)

        sanitized_path = sanitize_path(path)
        key = f"ratelimit:{user_id}:{sanitized_path}:{window_start}"

        try:
            current_count = await client.incr(key)
            if current_count == 1:
                await client.expire(key, period)
        except Exception:
            logger.exception(f"Error checking rate limit for user {user_id} on path {path}")
            raise
        return current_count > limit


async def check_sliding_window(redis: Redis, key: str, limit: int, period: int) -> bool:
    """Sliding-window-log rate limit using a sorted set.

    Returns True if the request is rate limited.

    The trim/add/count/expire sequence runs in a single MULTI transaction so that
    concurrent requests cannot all observe ``count < limit`` and slip through.
    Each member gets a random suffix: two requests with the same timestamp used to
    share one member (``str(now)``) and were counted only once.
    """
    now = datetime.now(UTC).timestamp()
    member = f"{now}:{uuid.uuid4().hex}"

    async with redis.pipeline(transaction=True) as pipe:
        pipe.zremrangebyscore(key, "-inf", now - period)
        pipe.zadd(key, {member: now})
        pipe.zcard(key)
        pipe.expire(key, period + 10)
        _, _, request_count, _ = await pipe.execute()

    if request_count > limit:
        # Rejected requests must not consume a slot in the window.
        await redis.zrem(key, member)
        return True
    return False


async def async_get_rate_limit_redis() -> AsyncGenerator[Redis | None, None]:
    """FastAPI dependency yielding the dedicated rate-limit Redis client (None if not initialized)."""
    yield RateLimiter().client


rate_limiter = RateLimiter()
