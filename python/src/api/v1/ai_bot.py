import json
import logging
import hashlib
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from redis import Redis
from pydantic import BaseModel, Field, field_validator

from ...core.ai.ollama_client import OllamaClient
from ...core.ai.enhancement_service import TextEnhancementService, EnhancementStyle, EnhancementTone
from ...api.dependencies import get_current_user, rate_limiter_dependency
from ...core.config import settings
from ...core.utils.cache import async_get_redis

logger = logging.getLogger(__name__)

# Global service instance
_enhancement_service: TextEnhancementService | None = None

class EnhanceRequest(BaseModel):
    """Request to enhance a single message"""
    message: str = Field(..., min_length=3, max_length=2000)
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL
    
    @field_validator('message')
    @classmethod
    def validate_message(cls, v: str) -> str:
        v = v.strip()
        if not v:
            raise ValueError("Message cannot be empty")
        return v

class EnhanceResponse(BaseModel):
    """Response with enhanced message"""
    original: str
    enhanced: str
    style: str
    tone: str
    improvements: list[str]
    metadata: dict
    cached: bool = False


class BatchEnhanceRequest(BaseModel):
    """Request to enhance multiple messages"""
    messages: list[str] = Field(..., min_length=1, max_length=5)
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL
    
    @field_validator('messages')
    @classmethod
    def validate_messages(cls, v: list[str]) -> list[str]:
        if not v:
            raise ValueError("At least one message required")
        if len(v) > 5:
            raise ValueError("Maximum 5 messages per batch")
        return [msg.strip() for msg in v if msg.strip()]

class BatchEnhanceResponse(BaseModel):
    """Response with multiple enhanced messages"""
    results: list[dict]
    total: int
    successful: int
    failed: int
    style: str
    tone: str
    total_processing_time_ms: float


class ModelInfoResponse(BaseModel):
    """Model information response"""
    status: str
    model: str
    base_url: str
    available_models: list[str]
    health: bool

def get_enhancement_service() -> TextEnhancementService:
    """Get or create the enhancement service singleton"""
    global _enhancement_service
    
    if _enhancement_service is None:
        ollama = OllamaClient(
            base_url=settings.OLLAMA_BASE_URL,
            model=settings.OLLAMA_MODEL,
            timeout=settings.OLLAMA_TIMEOUT,
            max_tokens=settings.OLLAMA_MAX_TOKENS,
            temperature=settings.OLLAMA_TEMPERATURE,
        )
        
        _enhancement_service = TextEnhancementService(
            ollama_client=ollama,
            enable_fallback=settings.ENABLE_RULE_BASED_FALLBACK,
        )
        
        logger.info(f"Enhancement service initialized with model: {settings.OLLAMA_MODEL}")
    
    return _enhancement_service


async def check_ollama_health() -> bool:
    """Check if Ollama service is healthy"""
    try:
        service = get_enhancement_service()
        return await service.ollama.health_check()
    except Exception as e:
        logger.error(f"Ollama health check failed: {e}")
        return False


def _build_cache_key(user_id: str, message: str, style: str, tone: str) -> str:
    """Build Redis cache key for enhancement"""
    content = f"{message}:{style}:{tone}".encode('utf-8')
    hash_digest = hashlib.sha256(content).hexdigest()
    return f"enhance:v2:{user_id}:{hash_digest}"


async def _get_cached_result(
    redis: Redis,
    user_id: str,
    message: str,
    style: str,
    tone: str
) -> dict | None:
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
    ttl: int = None
) -> None:
    """Cache enhancement result"""
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

router = APIRouter(prefix="/ai", tags=["ai-enhancement"])

@router.get("/health", response_model=dict)
async def health_check():
    """
    Check if AI enhancement service is healthy.
    """
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
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="AI service unavailable"
        )


@router.post("/enhance", response_model=EnhanceResponse)
async def enhance_message(
    req: EnhanceRequest,
    # user: Annotated[dict, Depends(get_current_user)],
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
    user_id = "TEST"
    
    # Check cache first
    cached = await _get_cached_result(
        redis, user_id, req.message, req.style.value, req.tone.value
    )
    
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
        
        # Cache result
        await _cache_result(
            redis, user_id, req.message, req.style.value, req.tone.value,
            enhanced, improvements, metadata
        )
        
        logger.info(
            f"Enhanced message for user {user_id}",
            extra={
                "style": req.style.value,
                "tone": req.tone.value,
                "method": metadata.get("method"),
                "processing_time_ms": metadata.get("processing_time_ms"),
            }
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
        
    except Exception as e:
        logger.error(f"Enhancement failed for user {user_id}: {e}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to enhance message"
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
    import time
    start_time = time.time()
    user_id = user["id"]
    
    results = []
    successful = 0
    failed = 0
    
    service = get_enhancement_service()
    
    for message in req.messages:
        if not message or len(message.strip()) < 3:
            results.append({
                "original": message,
                "enhanced": message,
                "error": "Message too short",
                "cached": False,
            })
            failed += 1
            continue
        
        # Check cache
        cached = await _get_cached_result(
            redis, user_id, message, req.style.value, req.tone.value
        )
        
        if cached:
            results.append({
                "original": message,
                "enhanced": cached["enhanced"],
                "improvements": cached["improvements"],
                "cached": True,
            })
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
                redis, user_id, message, req.style.value, req.tone.value,
                enhanced, improvements, metadata
            )
            
            results.append({
                "original": message,
                "enhanced": enhanced,
                "improvements": improvements,
                "metadata": metadata,
                "cached": False,
            })
            successful += 1
            
        except Exception as e:
            logger.error(f"Batch enhancement failed for message: {e}")
            results.append({
                "original": message,
                "enhanced": message,
                "error": str(e),
                "cached": False,
            })
            failed += 1
    
    processing_time = (time.time() - start_time) * 1000
    
    logger.info(
        f"Batch enhancement complete for user {user_id}",
        extra={
            "total": len(results),
            "successful": successful,
            "failed": failed,
            "processing_time_ms": round(processing_time, 2),
        }
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
                "description": "Business-appropriate with formal language"
            },
            {
                "value": "casual",
                "label": "Casual",
                "description": "Relaxed and conversational"
            },
            {
                "value": "concise",
                "label": "Concise",
                "description": "Brief and to the point"
            },
            {
                "value": "friendly",
                "label": "Friendly",
                "description": "Warm and approachable"
            },
            {
                "value": "formal",
                "label": "Formal",
                "description": "Sophisticated and respectful"
            }
        ],
        "tones": [
            {
                "value": "confident",
                "label": "Confident",
                "description": "Assertive and certain"
            },
            {
                "value": "polite",
                "label": "Polite",
                "description": "Courteous and respectful"
            },
            {
                "value": "neutral",
                "label": "Neutral",
                "description": "Balanced and objective"
            },
            {
                "value": "enthusiastic",
                "label": "Enthusiastic",
                "description": "Energetic and positive"
            }
        ]
    }
