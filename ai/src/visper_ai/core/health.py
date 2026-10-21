import asyncio
import logging

from redis.asyncio import Redis

LOGGER = logging.getLogger(__name__)

REDIS_HEALTH_TIMEOUT_SECONDS = 2.0


async def check_redis_health(redis: Redis) -> bool:
    try:
        # Bound the probe: a stalled TCP connection must not hang the readiness check.
        await asyncio.wait_for(redis.ping(), timeout=REDIS_HEALTH_TIMEOUT_SECONDS)
        return True
    except Exception as e:
        LOGGER.error(f"Redis health check failed: {type(e).__name__}: {e}")
        return False
