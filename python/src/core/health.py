import logging
from redis.asyncio import Redis

LOGGER = logging.getLogger(__name__)

async def check_redis_health(redis: Redis) -> bool:
    try:
        await redis.ping()
        return True
    except Exception as e:
        LOGGER.exception(f"Redis health check failed with error: {e}")
        return False
