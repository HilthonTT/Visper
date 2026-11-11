import hashlib
import json
import logging
import time
import uuid
from datetime import UTC, datetime
from typing import Annotated

import httpx
from fastapi import APIRouter, BackgroundTasks, Depends, HTTPException, Response, status
from redis.asyncio import Redis

from ...api.dependencies import get_current_user, rate_limiter_dependency
from ...core.ai.enhancement_service import EnhancementStyle, EnhancementTone, TextEnhancementService
from ...core.ai.ollama_client import OllamaClient
from ...core.config import settings
from ...core.utils.cache import async_get_redis
from ...schemas.ai_bot import (
    AsyncEnhanceRequest,
    AsyncEnhanceResponse,
    BatchEnhanceRequest,
    BatchEnhanceResponse,
    EnhanceRequest,
    EnhanceResponse,
    ModelInfoResponse,
    TaskStatusResponse,
)

logger = logging.getLogger(__name__)

# Global service instance
_enhancement_service: TextEnhancementService | None = None

CACHE_KEY_VERSION = "v3"
WEBHOOK_TIMEOUT = httpx.Timeout(5.0)


def get_enhancement_service() -> TextEnhancementService:
    """Get or create the enhancement service singleton"""
    global _enhancement_service

    if _enhancement_service is None:
        ollama = OllamaClient(
            base_url=settings.OLLAMA_BASE_URL,
            model=settings.OLLAMA_MODEL,
            timeout=settings.OLLAMA_TIMEOUT,
            connect_timeout=settings.OLLAMA_CONNECT_TIMEOUT,
            max_tokens=settings.OLLAMA_MAX_TOKENS,
            temperature=settings.OLLAMA_TEMPERATURE,
        )

        _enhancement_service = TextEnhancementService(
            ollama_client=ollama,
            enable_fallback=settings.ENABLE_RULE_BASED_FALLBACK,
        )

        logger.info(f"Enhancement service initialized with model: {settings.OLLAMA_MODEL}")

    return _enhancement_service


async def close_enhancement_service() -> None:
    """Close the Ollama HTTP client (called on app shutdown)."""
    global _enhancement_service
    if _enhancement_service is not None:
        await _enhancement_service.ollama.close()
        _enhancement_service = None


async def check_ollama_health() -> bool:
    """Check if Ollama service is healthy"""
    try:
        service = get_enhancement_service()
        return await service.ollama.health_check()
    except Exception as e:
        logger.error(f"Ollama health check failed: {e}")
        return False


def _now_iso() -> str:
    return datetime.now(UTC).isoformat()


def _build_cache_key(user_id: str, message: str, style: str, tone: str) -> str:
    """Build Redis cache key for enhancement.

    The hashed payload is JSON-encoded (unambiguous field boundaries) and includes the
    model, so switching OLLAMA_MODEL does not serve answers produced by another model.
    """
    content = json.dumps([settings.OLLAMA_MODEL, style, tone, message], ensure_ascii=False).encode("utf-8")
    hash_digest = hashlib.sha256(content).hexdigest()
    return f"enhance:{CACHE_KEY_VERSION}:{user_id}:{hash_digest}"


def _build_task_key(task_id: str) -> str:
    """Build Redis key for task status"""
    return f"enhance:task:{task_id}"


async def _get_cached_result(redis: Redis, user_id: str, message: str, style: str, tone: str) -> dict | None:
    """Get cached enhancement result"""
    try:
        cache_key = _build_cache_key(user_id, message, style, tone)
        cached = await redis.get(cache_key)
        if cached:
            return json.loads(cached)
    except Exception as e:
        logger.warning(f"Cache get failed: {e}")
    return None


async def _cache_result(
    redis: Redis,
    user_id: str,
    message: str,
    style: str,
    tone: str,
    enhanced: str,
    improvements: list[str],
    metadata: dict,
    ttl: int | None = None,
) -> None:
    """Cache enhancement result.

    Rule-based fallback output is not cached: otherwise a short Ollama outage would pin
    degraded results for ENHANCEMENT_CACHE_TTL even after the model is back.
    """
    if metadata.get("method") != "llm":
        return
    try:
        cache_key = _build_cache_key(user_id, message, style, tone)
        cache_data = {
            "enhanced": enhanced,
            "improvements": improvements,
            "metadata": metadata,
        }

        ttl = ttl or settings.ENHANCEMENT_CACHE_TTL
        await redis.set(cache_key, json.dumps(cache_data), ex=ttl)
    except Exception as e:
        logger.warning(f"Cache set failed: {e}")


async def _store_task_status(
    redis: Redis,
    task_id: str,
    status: str,
    data: dict,
    ttl: int = 3600,  # 1 hour
) -> None:
    """Store task status in Redis"""
    try:
        task_key = _build_task_key(task_id)
        task_data = {
            "task_id": task_id,
            "status": status,
            "created_at": _now_iso(),
            **data,
        }
        await redis.set(task_key, json.dumps(task_data), ex=ttl)
    except Exception as e:
        logger.error(f"Failed to store task status: {e}")


async def _get_task_status(redis: Redis, task_id: str) -> dict | None:
    """Get task status from Redis"""
    try:
        task_key = _build_task_key(task_id)
        task_data = await redis.get(task_key)
        if task_data:
            return json.loads(task_data)
    except Exception as e:
        logger.error(f"Failed to get task status: {e}")
    return None


async def _notify_webhook(callback_url: str | None, payload: dict) -> None:
    if not callback_url:
        return
    try:
        async with httpx.AsyncClient(timeout=WEBHOOK_TIMEOUT) as client:
            await client.post(callback_url, json=payload)
    except httpx.HTTPError as e:
        logger.warning(f"Failed to call webhook: {e}")


async def _process_enhancement_background(
    redis: Redis,
    task_id: str,
    user_id: str,
    message: str,
    style: EnhancementStyle,
    tone: EnhancementTone,
    callback_url: str | None = None,
    created_at: str | None = None,
) -> None:
    """Process enhancement in background"""
    created_at = created_at or _now_iso()
    base = {"user_id": user_id, "created_at": created_at}
    try:
        # Update status to processing
        await _store_task_status(redis, task_id, "processing", {**base, "original": message})

        # Check cache
        cached = await _get_cached_result(redis, user_id, message, style.value, tone.value)

        if cached:
            enhanced = cached["enhanced"]
            improvements = cached["improvements"]
            metadata = cached["metadata"]
        else:
            # Perform enhancement
            service = get_enhancement_service()

            enhanced, metadata = await service.enhance(message=message, style=style, tone=tone)

            improvements = service.analyze_improvements(
                original=message,
                enhanced=enhanced,
                style=style.value,
                tone=tone.value,
            )

            # Cache result
            await _cache_result(redis, user_id, message, style.value, tone.value, enhanced, improvements, metadata)

            logger.info(
                f"Background enhancement completed for task {task_id}",
                extra={
                    "user_id": user_id,
                    "method": metadata.get("method"),
                    "processing_time_ms": metadata.get("processing_time_ms"),
                },
            )

        # Store completed status
        await _store_task_status(
            redis,
            task_id,
            "completed",
            {
                **base,
                "completed_at": _now_iso(),
                "original": message,
                "enhanced": enhanced,
                "style": style.value,
                "tone": tone.value,
                "improvements": improvements,
                "metadata": metadata,
                "cached": bool(cached),
            },
        )
    except Exception as e:
        logger.error(f"Background enhancement failed for task {task_id}: {e}", exc_info=True)

        # Store failed status
        await _store_task_status(
            redis,
            task_id,
            "failed",
            {**base, "completed_at": _now_iso(), "error": "Enhancement failed"},
        )
        await _notify_webhook(callback_url, {"task_id": task_id, "status": "failed", "error": "Enhancement failed"})
        return

    await _notify_webhook(callback_url, {"task_id": task_id, "status": "completed"})


router = APIRouter(prefix="/ai", tags=["ai-enhancement"])


@router.get("/health", response_model=dict)
async def health_check(response: Response):
    """
    Check if AI enhancement service is healthy.

    Always 200 so the Go SDK can read `status`; `status` is "healthy" only when Ollama answers.
    """
    response.headers["Cache-Control"] = "no-store"
    is_healthy = await check_ollama_health()

    return {
        "status": "healthy" if is_healthy else "unhealthy",
        "model": settings.OLLAMA_MODEL,
        "base_url": settings.OLLAMA_BASE_URL,
    }


@router.get("/model-info", response_model=ModelInfoResponse)
async def get_model_info():
    """
    Get information about the AI model.
    """
    try:
        service = get_enhancement_service()

        # Get available models
        available = await service.ollama.list_models()

        # Check health
        is_healthy = await service.ollama.health_check()

        return ModelInfoResponse(
            status="ready" if is_healthy else "unavailable",
            model=settings.OLLAMA_MODEL,
            base_url=settings.OLLAMA_BASE_URL,
            available_models=available,
            health=is_healthy,
        )
    except Exception as e:
        logger.error(f"Failed to get model info: {e}")
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="AI service unavailable") from e


@router.post("/enhance/async", response_model=AsyncEnhanceResponse, status_code=status.HTTP_202_ACCEPTED)
async def enhance_message_async(
    req: AsyncEnhanceRequest,
    background_tasks: BackgroundTasks,
    redis: Annotated[Redis, Depends(async_get_redis)],
    user: Annotated[dict, Depends(get_current_user)],
    _: Annotated[None, Depends(rate_limiter_dependency)],
):
    """
    Enhance a message asynchronously (non-blocking - returns immediately).

    Returns a task_id that can be used to check the status via GET /ai/enhance/status/{task_id}.

    Optional webhook: Provide a callback_url to receive a POST request when enhancement completes.
    """
    user_id = user["id"]

    # Generate task ID
    task_id = str(uuid.uuid4())
    created_at = _now_iso()

    # Store initial pending status
    await _store_task_status(
        redis,
        task_id,
        "pending",
        {"user_id": user_id, "created_at": created_at, "original": req.message},
    )

    # Schedule background task. `redis` is the shared pooled client, so it is still
    # usable after the response (and after dependency teardown) when the task runs.
    background_tasks.add_task(
        _process_enhancement_background,
        redis=redis,
        task_id=task_id,
        user_id=user_id,
        message=req.message,
        style=req.style,
        tone=req.tone,
        callback_url=str(req.callback_url) if req.callback_url else None,
        created_at=created_at,
    )

    logger.info(f"Scheduled async enhancement task {task_id} for user {user_id}")

    return AsyncEnhanceResponse(
        task_id=task_id,
        status="pending",
        message="Enhancement task queued successfully",
        estimated_time_seconds=2,
    )


@router.get("/enhance/status/{task_id}", response_model=TaskStatusResponse)
async def get_enhancement_status(
    task_id: str,
    response: Response,
    redis: Annotated[Redis, Depends(async_get_redis)],
    user: Annotated[dict, Depends(get_current_user)],
):
    """
    Check the status of an async enhancement task.

    Returns the current status and result (if completed). Only the task owner may read it.
    """
    # Status changes while polling: must not get the default `public, max-age` header.
    response.headers["Cache-Control"] = "no-store"

    task_data = await _get_task_status(redis, task_id)

    if not task_data or task_data.get("user_id") != user["id"]:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=f"Task {task_id} not found")
    return TaskStatusResponse(**task_data)


@router.post("/enhance", response_model=EnhanceResponse)
async def enhance_message(
    req: EnhanceRequest,
    user: Annotated[dict, Depends(get_current_user)],
    redis: Annotated[Redis, Depends(async_get_redis)],
    _: Annotated[None, Depends(rate_limiter_dependency)],
):
    """
    Enhance a single message with AI.

    The AI will rewrite the message according to the specified style and tone
    while preserving the original meaning and intent.

    **Styles:**
    - `professional`: Business-appropriate, formal language
    - `casual`: Relaxed, conversational tone
    - `concise`: Brief and to the point
    - `friendly`: Warm and approachable
    - `formal`: Sophisticated and respectful

    **Tones:**
    - `confident`: Assertive and certain
    - `polite`: Courteous and respectful
    - `neutral`: Balanced and objective
    - `enthusiastic`: Energetic and positive
    """
    user_id = user["id"]

    # Check cache first
    cached = await _get_cached_result(redis, user_id, req.message, req.style.value, req.tone.value)

    if cached:
        logger.info(f"Cache hit for user {user_id}")
        return EnhanceResponse(
            original=req.message,
            enhanced=cached["enhanced"],
            style=req.style.value,
            tone=req.tone.value,
            improvements=cached["improvements"],
            metadata=cached["metadata"],
            cached=True,
        )

    # Enhance with AI
    try:
        service = get_enhancement_service()

        enhanced, metadata = await service.enhance(
            message=req.message,
            style=req.style,
            tone=req.tone,
        )

        improvements = service.analyze_improvements(
            original=req.message,
            enhanced=enhanced,
            style=req.style.value,
            tone=req.tone.value,
        )
    except Exception as e:
        logger.error(f"Enhancement failed for user {user_id}: {e}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR, detail="Failed to enhance message"
        ) from e

    # Cache result
    await _cache_result(redis, user_id, req.message, req.style.value, req.tone.value, enhanced, improvements, metadata)

    logger.info(
        f"Enhanced message for user {user_id}",
        extra={
            "style": req.style.value,
            "tone": req.tone.value,
            "method": metadata.get("method"),
            "processing_time_ms": metadata.get("processing_time_ms"),
        },
    )

    return EnhanceResponse(
        original=req.message,
        enhanced=enhanced,
        style=req.style.value,
        tone=req.tone.value,
        improvements=improvements,
        metadata=metadata,
        cached=False,
    )


@router.post("/enhance/batch", response_model=BatchEnhanceResponse)
async def enhance_messages_batch(
    req: BatchEnhanceRequest,
    user: Annotated[dict, Depends(get_current_user)],
    redis: Annotated[Redis, Depends(async_get_redis)],
    _: Annotated[None, Depends(rate_limiter_dependency)],
):
    """
    Enhance multiple messages in a single request.

    Maximum 5 messages per batch. Each message is processed independently,
    so if some fail, others will still be enhanced.
    """
    start_time = time.perf_counter()
    user_id = user["id"]

    results = []
    successful = 0
    failed = 0

    service = get_enhancement_service()

    for message in req.messages:
        if len(message) < settings.MIN_MESSAGE_LENGTH:
            results.append(
                {
                    "original": message,
                    "enhanced": message,
                    "error": "Message too short",
                    "cached": False,
                }
            )
            failed += 1
            continue

        # Check cache
        cached = await _get_cached_result(redis, user_id, message, req.style.value, req.tone.value)

        if cached:
            results.append(
                {
                    "original": message,
                    "enhanced": cached["enhanced"],
                    "improvements": cached["improvements"],
                    "cached": True,
                }
            )
            successful += 1
            continue

        # Enhance
        try:
            enhanced, metadata = await service.enhance(
                message=message,
                style=req.style,
                tone=req.tone,
            )

            improvements = service.analyze_improvements(
                original=message,
                enhanced=enhanced,
                style=req.style.value,
                tone=req.tone.value,
            )

            # Cache
            await _cache_result(
                redis, user_id, message, req.style.value, req.tone.value, enhanced, improvements, metadata
            )

            results.append(
                {
                    "original": message,
                    "enhanced": enhanced,
                    "improvements": improvements,
                    "metadata": metadata,
                    "cached": False,
                }
            )
            successful += 1

        except Exception as e:
            logger.error(f"Batch enhancement failed for message: {e}")
            results.append(
                {
                    "original": message,
                    "enhanced": message,
                    "error": "Enhancement failed",
                    "cached": False,
                }
            )
            failed += 1

    processing_time = (time.perf_counter() - start_time) * 1000

    logger.info(
        f"Batch enhancement complete for user {user_id}",
        extra={
            "total": len(results),
            "successful": successful,
            "failed": failed,
            "processing_time_ms": round(processing_time, 2),
        },
    )

    return BatchEnhanceResponse(
        results=results,
        total=len(results),
        successful=successful,
        failed=failed,
        style=req.style.value,
        tone=req.tone.value,
        total_processing_time_ms=round(processing_time, 2),
    )


@router.get("/styles", response_model=dict)
async def list_styles():
    """
    List available enhancement styles with descriptions.
    """
    return {
        "styles": [
            {
                "value": "professional",
                "label": "Professional",
                "description": "Business-appropriate with formal language",
            },
            {"value": "casual", "label": "Casual", "description": "Relaxed and conversational"},
            {"value": "concise", "label": "Concise", "description": "Brief and to the point"},
            {"value": "friendly", "label": "Friendly", "description": "Warm and approachable"},
            {"value": "formal", "label": "Formal", "description": "Sophisticated and respectful"},
        ],
        "tones": [
            {"value": "confident", "label": "Confident", "description": "Assertive and certain"},
            {"value": "polite", "label": "Polite", "description": "Courteous and respectful"},
            {"value": "neutral", "label": "Neutral", "description": "Balanced and objective"},
            {"value": "enthusiastic", "label": "Enthusiastic", "description": "Energetic and positive"},
        ],
    }
