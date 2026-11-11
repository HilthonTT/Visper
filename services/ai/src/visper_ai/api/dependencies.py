import json
import logging
from typing import Annotated, Any

from fastapi import Depends, Request
from redis.asyncio import Redis

from ..core.config import settings
from ..core.exceptions.http_exceptions import RateLimitException, UnauthorizedException
from ..core.utils.cache import async_get_redis
from ..core.utils.rate_limit import async_get_rate_limit_redis, check_sliding_window
from ..schemas.rate_limit import sanitize_path

logger = logging.getLogger(__name__)

DEFAULT_LIMIT = settings.DEFAULT_RATE_LIMIT_LIMIT
DEFAULT_PERIOD = settings.DEFAULT_RATE_LIMIT_PERIOD

USER_CONTEXT_KEY = "user"
USER_ID_HEADER_KEY = "X-User-ID"


async def get_user_from_redis(redis: Redis, user_id: str) -> dict[str, Any] | None:
    """Get existing user from Redis (created by GO API)."""
    if not user_id:
        return None

    user_key = f"{settings.REDIS_KEY_PREFIX}user:{user_id}"
    user_data = await redis.get(user_key)

    if not user_data:
        return None

    return json.loads(user_data)


def get_user_id_from_request(request: Request) -> str | None:
    """Extract user ID from header or cookie."""
    if user_id := request.headers.get(USER_ID_HEADER_KEY):
        return user_id

    if user_id := request.cookies.get("user_id"):
        return user_id

    return None


def is_guest_user(user: dict[str, Any]) -> bool:
    """The Go API serialises the flag as ``isGuest``; accept ``is_guest`` too."""
    if "isGuest" in user:
        return bool(user["isGuest"])
    return bool(user.get("is_guest", True))


def resolve_rate_limit(user: dict[str, Any] | None) -> int:
    """Return the request limit for this caller (registered > guest > anonymous)."""
    if user is None:
        limit = settings.ANONYMOUS_RATE_LIMIT_LIMIT
        return limit if limit is not None else max(1, DEFAULT_LIMIT // 4)
    if is_guest_user(user):
        limit = settings.GUEST_RATE_LIMIT_LIMIT
        return limit if limit is not None else max(1, DEFAULT_LIMIT // 2)
    return DEFAULT_LIMIT


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
    except Exception as e:
        logger.error(f"Failed to get user from Redis: {e}, user_id: {user_id}")
        raise UnauthorizedException("Failed to retrieve user session") from e

    if not user or "id" not in user:
        logger.warning(f"User not found in Redis: {user_id}")
        raise UnauthorizedException("User session expired or invalid. Please access via main API first.")

    request.state.user = user  # Store in request state
    logger.debug(f"Retrieved user: {user['id']}, username: {user.get('username', 'N/A')}")
    return user


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
        if user and "id" in user:
            request.state.user = user
            return user
        return None
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
    Check if user has exceeded rate limit using Redis (sliding window, sorted set).

    Returns True if rate limited (exceeded), False otherwise.
    """
    key = f"rate_limit:{user_id}:{path}"
    return await check_sliding_window(redis, key, limit=limit, period=period)


async def rate_limiter_dependency(
    request: Request,
    redis: Annotated[Redis | None, Depends(async_get_rate_limit_redis)],
    user: Annotated[dict | None, Depends(get_optional_user)],
) -> None:
    if hasattr(request.app.state, "initialization_complete"):
        await request.app.state.initialization_complete.wait()

    path = sanitize_path(request.url.path)
    # Namespaced so a user id can never collide with an IP; anonymous callers fall back to IP.
    client_host = request.client.host if request.client else "unknown"
    user_id = f"user:{user['id']}" if user else f"ip:{client_host}"
    limit = resolve_rate_limit(user)
    period = DEFAULT_PERIOD

    if redis is None:
        # Fail open - don't block requests when the limiter is unavailable
        logger.error("Rate limiter Redis client not initialized; skipping rate limit")
        return

    try:
        is_limited = await check_rate_limit(
            redis=redis,
            user_id=user_id,
            limit=limit,
            period=period,
            path=path,
        )
    except Exception as e:
        # Fail open - don't block requests on rate limiter errors
        logger.error(f"Error checking rate limit: {e}", extra={"user_id": user_id, "path": path})
        return

    if is_limited:
        logger.warning(
            "Rate limit exceeded",
            extra={
                "user_id": user_id,
                "path": path,
                "limit": limit,
                "period": period,
            },
        )
        raise RateLimitException(f"Rate limit exceeded. Maximum {limit} requests per {period} seconds allowed.")
