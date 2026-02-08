
import json
import logging
import time
from fastapi import APIRouter, Depends, HTTPException, Request
from redis import Redis
from transformers import pipeline
from ...api.dependencies import get_current_user, rate_limiter_dependency
from ...core.config import settings
from ...core.utils.cache import async_get_redis
from ...schemas.ai_bot import EnhanceMessageRequest, EnhanceMessageResponse, BatchEnhanceRequest

logger = logging.getLogger(__name__)
router = APIRouter(tags=["ai"])

_model_cache = {}
_model_load_lock = False

def get_enhancement_model():
    """Get or create text enhancement model instance"""
    global _model_cache, _model_load_lock
    
    if 'enhancer' in _model_cache:
        return _model_cache['enhancer']
    if _model_load_lock:
        raise HTTPException(status_code=503, detail="Model is currently loading")
    try:
        _model_load_lock = True
        
        model_name = getattr(settings, 'ENHANCEMENT_MODEL', 'google/flan-t5-small')
        
        enhancer = pipeline(
            'text-generation',
            model=model_name,
            device='cpu',  # Use 'cuda' if GPU available
            model_kwargs={'cache_dir': settings.MODEL_CACHE_DIR}
        )
         
        enhancer('test message', max_length=50, do_sample=False)
        
        _model_cache['enhancer'] = enhancer
        logger.info(f"Enhancement model loaded: {model_name}")
        return enhancer
    except Exception as e:
        logger.error(f"Failed to load enhancement model: {e}")
        raise HTTPException(status_code=503, detail="Enhancement model is unavailable")
    finally:
        _model_load_lock = False
        
def _build_enhancement_prompt(message: str, style: str, tone: str) -> str:
    """Build prompt for the enhancement model"""
    style_instructions = {
        "professional": "Make this message more professional and formal",
        "casual": "Make this message more casual and friendly",
        "concise": "Make this message shorter and more concise",
        "detailed": "Make this message more detailed and thorough",
        "friendly": "Make this message warmer and more friendly"
    }
    
    tone_instructions = {
        "neutral": "with a neutral tone",
        "confident": "with a confident tone",
        "empathetic": "with an empathetic tone",
        "persuasive": "with a persuasive tone"
    }
    
    instruction = f"{style_instructions.get(style, 'Improve this message')} {tone_instructions.get(tone, '')}"
    
    prompt = f"{instruction}: {message}"
    return prompt

def _analyze_improvements(original: str, enhanced: str, style: str, tone: str) -> list[str]:
    """Analyze what improvements were made"""
    improvements = []
    
    if len(enhanced) > len(original):
        improvements.append("Expanded message with more detail")
    elif len(enhanced) < len(original):
        improvements.append("Made message more concise")
        
    if enhanced[0].isupper() and not original[0].isupper():
        improvements.append("Capitalized first letter")
    
    if enhanced.endswith(('.', '!', '?')) and not original.endswith(('.', '!', '?')):
        improvements.append("Added proper punctuation")
        
    original_words = set(original.lower().split())
    enhanced_words = set(enhanced.lower().split())
    new_words = len(enhanced_words - original_words)
    
    if new_words > 0:
        improvements.append(f"Enhanced vocabulary with {new_words} new words")
        
    improvements.append(f"Applied {style} style")
    improvements.append(f"Used {tone} tone")
    
    return improvements

async def _generate_enhancement(
    message: str,
    style: str,
    tone: str,
    preserve_intent: bool
) -> tuple[str, list[str]]:
    """
    Generate enhanced message using local model.
    """
    model = get_enhancement_model()
    
    prompt = _build_enhancement_prompt(message, style, tone)
    
    # Generate enhancement
    result = model(
        prompt,
        max_length=150,  # Adjust based on your needs
        min_length=10,
        do_sample=True,
        temperature=0.7,
        top_p=0.9,
        num_return_sequences=1
    )
    
    enhanced = result[0]['generated_text'].strip()
    
    # Fallback to original if generation failed
    if not enhanced or len(enhanced) < 3:
        enhanced = message
    
    # Analyze improvements
    improvements = _analyze_improvements(message, enhanced, style, tone)
    
    return enhanced, improvements

async def _get_cached_enhancement(
    redis: Redis,
    user_id: str,
    message: str,
    style: str,
    tone: str
) -> dict | None:
    """Get cached enhancement from Redis"""
    import hashlib
    
    # Create cache key from message + parameters
    cache_content = f"{message}:{style}:{tone}".encode('utf-8')
    message_hash = hashlib.sha256(cache_content).hexdigest()
    cache_key = f"enhance:{user_id}:{message_hash}"
    
    cached = await redis.get(cache_key)
    if cached:
        return json.loads(cached)
    return None


async def _cache_enhancement(
    redis: Redis,
    user_id: str,
    message: str,
    style: str,
    tone: str,
    enhanced: str,
    improvements: list[str],
    ttl: int = 3600
) -> None:
    """Cache enhancement result in Redis"""
    import hashlib
    
    cache_content = f"{message}:{style}:{tone}".encode('utf-8')
    message_hash = hashlib.sha256(cache_content).hexdigest()
    cache_key = f"enhance:{user_id}:{message_hash}"
    
    cache_data = {
        "enhanced": enhanced,
        "improvements": improvements
    }
    
    await redis.set(cache_key, json.dumps(cache_data), ex=ttl)

@router.post("/enhance", response_model=EnhanceMessageResponse)
async def enhance_message(
    request: Request,
    req: EnhanceMessageRequest,
    user: dict = Depends(get_current_user),
    redis: Redis = Depends(rate_limiter_dependency),
    _: None = Depends(rate_limiter_dependency)):
    """
    Enhance message with AI assistance using local model.
    
    - **message**: Original message to enhance (5-500 characters)
    - **style**: Enhancement style (professional, casual, concise, detailed, friendly)
    - **tone**: Desired tone (neutral, confident, empathetic, persuasive)
    - **preserve_intent**: Keep original message intent
    
    Returns enhanced version with list of improvements made.
    """
    start_time = time.time()
    user_id = user["id"]
    
    try:
        cached = await _get_cached_enhancement(
            redis=redis,
            user_id=user_id,
            message=req.message,
            style=req.style,
            tone=req.tone,
        )
        
        if cached:
            processing_time = (time.time() - start_time) * 1000
            logger.info(
                "Cache hit for message enhancement",
                extra={"user_id": user_id, "style": req.style}
            )
            return EnhanceMessageResponse(
                original=req.message,
                enhanced=cached["enhanced"],
                style=req.style,
                tone=req.tone,
                improvements=cached["improvements"],
                processing_time_ms=round(processing_time, 2),
                cached=True
            )
        
        enhanced, improvements = await _generate_enhancement(
            message=req.message,
            style=req.style,
            tone=req.tone,
            preserve_intent=req.preserve_intent
        )
        
        await _cache_enhancement(
            redis=redis,
            user_id=user_id,
            message=req.message,
            style=req.style,
            tone=req.tone,
            enhanced=enhanced,
            improvements=improvements
        )
        
        processing_time = (time.time() - start_time) * 1000
        
        logger.info(
            "Message enhanced successfully",
            extra={
                "user_id": user_id,
                "style": req.style,
                "tone": req.tone,
                "original_length": len(req.message),
                "enhanced_length": len(enhanced),
                "processing_time_ms": round(processing_time, 2)
            }
        )
        
        return EnhanceMessageResponse(
            original=req.message,
            enhanced=enhanced,
            style=req.style,
            tone=req.tone,
            improvements=improvements,
            processing_time_ms=round(processing_time, 2),
            cached=False
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(
            f"Message enhancement failed: {e}",
            exc_info=True,
            extra={"user_id": user_id}
        )
        raise HTTPException(
            status_code=500,
            detail="Message enhancement failed"
        )
        

@router.post("/enhance/batch", response_model=dict)
async def enhance_messages_batch(
    request: Request,
    req: BatchEnhanceRequest,
    user: dict = Depends(get_current_user),
    redis: Redis = Depends(async_get_redis),
    _: None = Depends(rate_limiter_dependency)
):
    """
    Enhance multiple messages in a single request.
    
    Maximum 5 messages per batch (reduced for local model performance).
    """
    start_time = time.time()
    user_id = user["id"]
    
    try:
        results = []
        
        for msg in req.messages:
            if len(msg.strip()) < 5:
                results.append({
                    "original": msg,
                    "enhanced": msg,
                    "error": "Message too short"
                })
                continue
            
            # Check cache
            cached = await _get_cached_enhancement(
                redis=redis,
                user_id=user_id,
                message=msg,
                style=req.style,
                tone=req.tone,
            )
            
            if cached:
                results.append({
                    "original": msg,
                    "enhanced": cached["enhanced"],
                    "cached": True
                })
            else:
                enhanced, improvements = await _generate_enhancement(
                    message=msg,
                    style=req.style,
                    tone=req.tone,
                    preserve_intent=True
                )
                
                await _cache_enhancement(
                    redis=redis,
                    user_id=user_id,
                    message=msg,
                    style=req.style,
                    tone=req.tone,
                    enhanced=enhanced,
                    improvements=improvements
                )
                
                results.append({
                    "original": msg,
                    "enhanced": enhanced,
                    "cached": False
                })
        
        processing_time = (time.time() - start_time) * 1000
        
        return {
            "results": results,
            "count": len(results),
            "style": req.style,
            "tone": req.tone,
            "processing_time_ms": round(processing_time, 2)
        }
        
    except Exception as e:
        logger.error(f"Batch enhancement failed: {e}", exc_info=True)
        raise HTTPException(
            status_code=500,
            detail="Batch enhancement failed"
        )

@router.get("/enhance/model-info")
async def get_model_info():
    """Get information about the loaded enhancement model"""
    try:
        model = get_enhancement_model()
        model_name = getattr(settings, 'ENHANCEMENT_MODEL', 'google/flan-t5-small')
        
        return {
            "model": model_name,
            "status": "loaded",
            "device": "cpu",  # or "cuda" if GPU
            "max_length": 500
        }
    except Exception as e:
        return {
            "status": "error",
            "error": str(e)
        }
        