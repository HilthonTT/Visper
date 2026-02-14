from datetime import UTC, datetime
from typing import Annotated, Any
from fastapi import Depends, Request
from redis import Redis

from ..core.utils.cache import async_get_redis
from ..schemas.rate_limit import sanitize_path
from ..core.logger import logging
from ..core.config import settings
from ..core.exceptions.http_exceptions import UnauthorizedException, RateLimitException

logger = logging.getLogger(__name__)

DEFAULT_LIMIT = settings.DEFAULT_RATE_LIMIT_LIMIT
DEFAULT_PERIOD = settings.DEFAULT_RATE_LIMIT_PERIOD

USER_CONTEXT_KEY = "user"
USER_ID_HEADER_KEY = "X-User-ID"

async def get_user_from_redis(redis: Redis, user_id: str) -> dict[str, Any] | None:
    """Get existing user from Redis (created by GO API)."""
    if not user_id:
        return None
    
    user_key = f"user:{user_id}"
    user_data = await redis.get(user_key)
    
    if not user_data:
        return None
    
    import json
    return json.loads(user_data)

def get_user_id_from_request(request: Request) -> str | None:
    """Extract user ID from header or cookie."""
    if user_id := request.headers.get("X-User-ID"):
        return user_id
    
    if user_id := request.cookies.get("user_id"):
        return user_id
    
    return None

async def get_current_user(
    request: Request,
    redis: Annotated[Redis, Depends(async_get_redis)],
) -> dict[str, Any]:
    """
    Get current user from Redis session (created by Go API).
    
    This dependency:
    1. Extracts user ID from headers or cookies
    2. Fetches user data from Redis
    3. Raises UnauthorizedException if user not found
    4. Stores user in request state for later access
    """
    user_id = get_user_id_from_request(request)
    
    if not user_id:
        logger.warning("No user ID found in request")
        raise UnauthorizedException("User session not found. Please access via main API first.")
    
    try:
        user = await get_user_from_redis(redis, user_id)
        
        if not user:
            logger.warning(f"User not found in Redis: {user_id}")
            raise UnauthorizedException("User session expired or invalid. Please access via main API first.")
        
        request.state.user = user  # Store in request state
        logger.debug(f"Retrieved user: {user['id']}, username: {user.get('username', 'N/A')}")
        return user
        
    except UnauthorizedException:
        raise
    except Exception as e:
        logger.error(f"Failed to get user from Redis: {e}, user_id: {user_id}")
        raise UnauthorizedException("Failed to retrieve user session")
    
async def get_optional_user(
    request: Request,
    redis: Annotated[Redis, Depends(async_get_redis)],
) -> dict[str, Any] | None:
    """
    Optional user dependency - returns None if user not found instead of raising exception.
    Useful for endpoints that work with or without authentication.
    """
    user_id = get_user_id_from_request(request)
    
    if not user_id:
        return None
    
    try:
        user = await get_user_from_redis(redis, user_id)
        if user:
            request.state.user = user
        return user
    except Exception as e:
        logger.error(f"Error retrieving optional user: {e}, user_id: {user_id}")
        return None

async def check_rate_limit(
    redis: Redis,
    user_id: str,
    path: str,
    limit: int,
    period: int,
) -> bool:
    """
    Check if user has exceeded rate limit using Redis.
    
    Uses sliding window algorithm with Redis sorted sets.
    
    Parameters
    ----------
    redis : Redis
        Redis client instance
    user_id : str
        Unique identifier for the user
    path : str
        Sanitized request path
    limit : int
        Maximum number of requests allowed in the time window
    period : int
        Time window in seconds
    
    Returns
    -------
    bool
        True if rate limited (exceeded), False otherwise
    """
    now = datetime.now(UTC).timestamp()
    window_start = now - period
    
    # Create unique key for this user+path combination
    key = f"rate_limit:{user_id}:{path}"
    
    # Remove old entries outside the time window
    await redis.zremrangebyscore(key, "-inf", window_start)
    
    # Count requests in current window
    request_count = await redis.zcard(key)
    
    if request_count >= limit:
        return True
    
    # Add current request with timestamp as score
    await redis.zadd(key, {str(now): now})
    
    # Set expiration on the key (cleanup)
    await redis.expire(key, period + 10)
    
    return False

async def rate_limiter_dependency(
    request: Request,
    redis: Annotated[Redis, Depends(async_get_redis)],
    user: dict | None = Depends(get_optional_user)) -> None:
    if hasattr(request.app.state, "initialization_complete"):
        await request.app.state.initialization_complete.wait()
        
    path = sanitize_path(request.url.path)
    if user:
        user_id = user["id"]
        if user.get("is_guest", True):
            limit = getattr(settings, "GUEST_RATE_LIMIT_LIMIT", DEFAULT_LIMIT // 2)  # Lower for guests
        else:
            limit = DEFAULT_LIMIT
    else:
        # Anonymous users (fallback to IP)
        user_id = request.client.host if request.client else "unknown"
        limit = getattr(settings, "ANONYMOUS_RATE_LIMIT_LIMIT", DEFAULT_LIMIT // 4)  # Lowest for anonymous
        
    period = DEFAULT_PERIOD
    
    try:
        is_limited = await check_rate_limit(
            redis=redis,
            user_id=user_id,
            limit=limit,
            period=period,
            path=path,
        )
        if is_limited:
            logger.warning(
                f"Rate limit exceeded",
                extra={
                    "user_id": user_id,
                    "path": path,
                    "limit": limit,
                    "period": period,
                }
            )
            raise RateLimitException(
                f"Rate limit exceeded. Maximum {limit} requests per {period} seconds allowed."
            )
    except RateLimitException:
        raise
    except Exception as e:
        logger.error(f"Error checking rate limit: {e}", extra={"user_id": user_id, "path": path})
        # Fail open - don't block requests on rate limiter errors
        pass

    