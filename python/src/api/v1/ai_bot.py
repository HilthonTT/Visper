import json
import logging
import time
import re
from fastapi import APIRouter, Depends, HTTPException, Request
from redis import Redis
from transformers import AutoTokenizer, AutoModelForSeq2SeqLM
from ...api.dependencies import get_current_user, rate_limiter_dependency
from ...core.config import settings
from ...core.utils.cache import async_get_redis
from ...schemas.ai_bot import EnhanceMessageRequest, EnhanceMessageResponse, BatchEnhanceRequest

logger = logging.getLogger(__name__)
router = APIRouter(tags=["ai"])

_model_cache = {}
_model_load_lock = False

DEFAULT_MODEL_NAME = "Vamsi/T5_Paraphrase_Paws"  

def get_enhancement_model():
    """Get or create text enhancement model instance"""
    global _model_cache, _model_load_lock
    
    if 'enhancer' in _model_cache:
        return _model_cache['enhancer']
    if _model_load_lock:
        raise HTTPException(status_code=503, detail="Model is currently loading")
    try:
        _model_load_lock = True
        
        model_name = getattr(settings, 'ENHANCEMENT_MODEL', DEFAULT_MODEL_NAME)
        
        tokenizer = AutoTokenizer.from_pretrained(
            model_name,
            cache_dir=settings.MODEL_CACHE_DIR
        )
        model = AutoModelForSeq2SeqLM.from_pretrained(
            model_name,
            cache_dir=settings.MODEL_CACHE_DIR
        )
        
        _model_cache['enhancer'] = {
            'model': model,
            'tokenizer': tokenizer,
            'name': model_name
        }
        
        # Warmup
        test_input = tokenizer("paraphrase: test message", return_tensors="pt")
        model.generate(**test_input, max_length=50)
        
        logger.info(f"Enhancement model loaded: {model_name}")
        return _model_cache['enhancer']
        
    except Exception as e:
        logger.error(f"Failed to load enhancement model: {e}")
        raise HTTPException(status_code=503, detail="Enhancement model is unavailable")
    finally:
        _model_load_lock = False

def _rule_based_enhancement(message: str, style: str, tone: str) -> str:
    """
    Fallback rule-based enhancement when model fails.
    This is now the PRIMARY method since local models are unreliable.
    """
    enhanced = message.strip()
    
    if style == "professional":
        # Comprehensive replacements using regex for word boundaries
        replacements = {
            r'\bu\b': 'you',
            r'\bur\b': 'your',
            r'\burs\b': 'yours',
            r'\bplz\b': 'please',
            r'\bpls\b': 'please',
            r'\bthx\b': 'thank you',
            r'\bthanx\b': 'thank you',
            r'\bty\b': 'thank you',
            r'\bnp\b': 'no problem',
            r'\bbro\b': '',
            r'\bdude\b': '',
            r'\bman\b': '',
            r'\bgonna\b': 'going to',
            r'\bwanna\b': 'want to',
            r'\bgotta\b': 'have to',
            r'\bkinda\b': 'kind of',
            r'\bsorta\b': 'sort of',
            r'\byeah\b': 'yes',
            r'\byep\b': 'yes',
            r'\bnah\b': 'no',
            r'\bnope\b': 'no',
            r'\bok\b': 'okay',
            r'\bk\b': 'okay',
            r'\bcuz\b': 'because',
            r'\bcause\b': 'because',
            r'\btho\b': 'though',
            r'\bthru\b': 'through',
            r'\bbtw\b': 'by the way',
            r'\bidk\b': "I don't know",
            r'\bimo\b': 'in my opinion',
            r'\bfyi\b': 'for your information',
            r'\basap\b': 'as soon as possible',
            r'\byo\b': 'hello',
            r'\bhey+\b': 'hello',  # Multiple y's
        }
        
        for pattern, replacement in replacements.items():
            enhanced = re.sub(pattern, replacement, enhanced, flags=re.IGNORECASE)
        
        # Remove multiple exclamation/question marks
        enhanced = re.sub(r'!+', '!', enhanced)
        enhanced = re.sub(r'\?+', '?', enhanced)
        
        # Clean up double spaces from removed words
        enhanced = re.sub(r'\s+', ' ', enhanced).strip()
        
        # Remove comma before empty space (from removed words like "bro, ")
        enhanced = re.sub(r',\s*,', ',', enhanced)
        enhanced = re.sub(r',\s*\.', '.', enhanced)
        enhanced = re.sub(r',\s*\?', '?', enhanced)
        enhanced = re.sub(r',\s*!', '!', enhanced)
        
        # Capitalize first letter
        if enhanced and enhanced[0].islower():
            enhanced = enhanced[0].upper() + enhanced[1:]
        
        # Add period if missing and doesn't end with punctuation
        if enhanced and not enhanced[-1] in '.!?':
            enhanced += '.'
    
    elif style == "casual":
        # Make it more casual (opposite of professional)
        if not enhanced.endswith(('.', '!', '?')):
            enhanced += '!'
    
    elif style == "concise":
        # Remove filler words
        fillers = [
            r'\blike\b', r'\bjust\b', r'\breally\b', r'\bvery\b', 
            r'\bactually\b', r'\bbasically\b', r'\bliterally\b',
            r'\bkind of\b', r'\bsort of\b'
        ]
        for filler in fillers:
            enhanced = re.sub(filler, '', enhanced, flags=re.IGNORECASE)
        enhanced = re.sub(r'\s+', ' ', enhanced).strip()
    
    elif style == "friendly":
        # Add friendly touch
        if not enhanced.endswith('!'):
            enhanced = enhanced.rstrip('.') + '!'
    
    # Apply tone modifications
    if tone == "confident" and style == "professional":
        # Remove hedging language
        hedge_words = [r'\bmaybe\b', r'\bperhaps\b', r'\bpossibly\b', r'\bI think\b']
        for hedge in hedge_words:
            enhanced = re.sub(hedge, '', enhanced, flags=re.IGNORECASE)
        enhanced = re.sub(r'\s+', ' ', enhanced).strip()
    
    return enhanced

def _build_enhancement_prompt(message: str, style: str, tone: str) -> str:
    """Build prompt for paraphrasing model"""
    # The Vamsi/T5_Paraphrase_Paws model expects "paraphrase: " prefix
    # We'll preprocess the message first with rules, then paraphrase
    preprocessed = _rule_based_enhancement(message, style, tone)
    return f"paraphrase: {preprocessed}"

def _analyze_improvements(original: str, enhanced: str, style: str, tone: str) -> list[str]:
    """Analyze what improvements were made"""
    improvements = []
    
    if enhanced.lower() == original.lower():
        improvements.append(f"Applied {style} style")
        improvements.append(f"Used {tone} tone")
        return improvements
    
    if len(enhanced) > len(original) * 1.2:
        improvements.append("Expanded message with more detail")
    elif len(enhanced) < len(original) * 0.8:
        improvements.append("Made message more concise")
    
    if enhanced[0].isupper() and not original[0].isupper():
        improvements.append("Capitalized first letter")
    
    if enhanced.endswith(('.', '!', '?')) and not original.endswith(('.', '!', '?')):
        improvements.append("Added proper punctuation")
    
    # Check for formal words
    informal_words = ['u', 'ur', 'plz', 'bro', 'dude', 'gonna', 'wanna', 'gotta', 'yo']
    removed_informal = any(re.search(rf'\b{word}\b', original.lower()) for word in informal_words) and \
                      not any(re.search(rf'\b{word}\b', enhanced.lower()) for word in informal_words)
    
    if removed_informal:
        improvements.append("Replaced informal language")
        
    original_words = set(original.lower().split())
    enhanced_words = set(enhanced.lower().split())
    new_words = len(enhanced_words - original_words)
    
    if new_words > 2:
        improvements.append("Enhanced vocabulary")
    
    if style not in [imp.lower() for imp in improvements]:
        improvements.append(f"Applied {style} style")
    if tone not in [imp.lower() for imp in improvements]:
        improvements.append(f"Used {tone} tone")
    
    return improvements

async def _generate_enhancement(
    message: str,
    style: str,
    tone: str,
    preserve_intent: bool
) -> tuple[str, list[str]]:
    """
    Generate enhanced message.
    Strategy: Use rule-based as PRIMARY, optionally enhance with model.
    """
    
    # ALWAYS apply rule-based enhancement first
    enhanced = _rule_based_enhancement(message, style, tone)
    
    # Only try model enhancement if the message is long enough and complex
    # (Models work better on longer text)
    if len(message.split()) >= 5:
        try:
            model_data = get_enhancement_model()
            model = model_data['model']
            tokenizer = model_data['tokenizer']
            
            # Use the rule-enhanced version as input to model
            prompt = f"paraphrase: {enhanced}"
            
            inputs = tokenizer(prompt, return_tensors="pt", max_length=512, truncation=True)
            
            outputs = model.generate(
                **inputs,
                max_length=150,
                num_beams=3,
                early_stopping=True,
                temperature=0.7,
                do_sample=False,  # Use greedy for consistency
            )
            
            model_output = tokenizer.decode(outputs[0], skip_special_tokens=True).strip()
            
            # Clean model output
            model_output = model_output.replace("paraphrase:", "").strip()
            
            # Only use model output if it's actually better (longer, different, valid)
            if (model_output and 
                len(model_output) >= len(message) * 0.7 and
                model_output.lower() != enhanced.lower() and
                len(model_output.split()) >= 3):
                enhanced = model_output
                
                # Re-apply style rules to model output (models can be inconsistent)
                enhanced = _rule_based_enhancement(enhanced, style, tone)
        
        except Exception as e:
            logger.warning(f"Model enhancement failed, using rule-based: {e}")
            # enhanced is already set to rule-based version
    
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
    redis: Redis = Depends(async_get_redis),
    _: None = Depends(rate_limiter_dependency)):
    """
    Enhance message with AI assistance using local model.
    """
    start_time = time.time()
    user_id = user['id']
    
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
        model_name = getattr(settings, 'ENHANCEMENT_MODEL', DEFAULT_MODEL_NAME)
        
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
