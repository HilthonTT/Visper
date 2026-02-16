import re
import logging
from enum import Enum

from .ollama_client import OllamaClient

logger = logging.getLogger(__name__)

class EnhancementStyle(str, Enum):
    """Text enhancement styles"""
    PROFESSIONAL = "professional"
    CASUAL = "casual"
    CONCISE = "concise"
    FRIENDLY = "friendly"
    FORMAL = "formal"
    
class EnhancementTone(str, Enum):
    """Text enhancement tones"""
    CONFIDENT = "confident"
    POLITE = "polite"
    NEUTRAL = "neutral"
    ENTHUSIASTIC = "enthusiastic"
    
class TextEnhancementService:
    """
    Service for enhancing text messages using Ollama LLM.
    
    Combines rule-based preprocessing with LLM enhancement for best results.
    """
    
    def __init__(self, ollama_client: OllamaClient, enable_fallback: bool = True):
        self.ollama = ollama_client
        self.enable_fallback = enable_fallback
        
    def _build_system_prompt(self, style: EnhancementStyle, tone: EnhancementTone) -> str:
        """Build system prompt for the enhancement task"""
        
        style_instructions = {
            EnhancementStyle.PROFESSIONAL: (
                "Rewrite the message in a professional business style. "
                "Remove slang, abbreviations, and informal language. "
                "Use proper grammar and punctuation. Maintain formality."
            ),
            EnhancementStyle.CASUAL: (
                "Rewrite the message in a casual, conversational style. "
                "Keep it natural and friendly. It's okay to be relaxed."
            ),
            EnhancementStyle.CONCISE: (
                "Rewrite the message to be more concise and to the point. "
                "Remove unnecessary words while preserving the core meaning. "
                "Be brief but complete."
            ),
            EnhancementStyle.FRIENDLY: (
                "Rewrite the message in a warm, friendly tone. "
                "Make it approachable and personable while staying appropriate."
            ),
            EnhancementStyle.FORMAL: (
                "Rewrite the message in a formal, respectful style. "
                "Use sophisticated vocabulary and complete sentences. "
                "Avoid contractions and maintain professional distance."
            ),
        }
        
        tone_instructions = {
            EnhancementTone.CONFIDENT: "Express confidence and certainty. Avoid hedging words like 'maybe', 'perhaps', 'I think'.",
            EnhancementTone.POLITE: "Be courteous and respectful. Use please/thank you where appropriate.",
            EnhancementTone.NEUTRAL: "Maintain an objective, balanced tone without strong emotion.",
            EnhancementTone.ENTHUSIASTIC: "Show energy and positivity. Use exclamation points where appropriate.",
        }
        
        return f"""You are a professional text editor. Your job is to enhance messages while preserving their core meaning.
Style: {style_instructions[style]}

Tone: {tone_instructions[tone]}

Rules:
1. ONLY output the enhanced message - no explanations, no meta-commentary
2. Preserve the core meaning and intent
3. Keep the length similar (don't make it 3x longer or shorter)
4. If the message is already well-written, make minimal changes
5. Fix obvious typos and grammar errors
6. Do not add new information or claims
7. Output ONLY the rewritten text"""
    

    def _rule_based_preprocess(self, message: str, style: EnhancementStyle) -> str:
        """
        Apply rule-based preprocessing before LLM enhancement.
        This catches common patterns and reduces LLM load.
        """
        text = message.strip()
        
        if style == EnhancementStyle.PROFESSIONAL or style == EnhancementStyle.FORMAL:
            # Expand common abbreviations
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
                r'\bcuz\b': 'because',
                r'\bcause\b': 'because',
                r'\btho\b': 'though',
                r'\bthru\b': 'through',
                r'\bbtw\b': 'by the way',
                r'\bidk\b': "I don't know",
                r'\bimo\b': 'in my opinion',
                r'\bfyi\b': 'for your information',
                r'\basap\b': 'as soon as possible',
            }
            for pattern, replacement in replacements.items():
                text = re.sub(pattern, replacement, text, flags=re.IGNORECASE)
                
            # Remove casual words
            casual_removals = [r'\bbro\b', r'\bdude\b', r'\bman\b', r'\byo\b']
            for pattern in casual_removals:
                text = re.sub(pattern, '', text, flags=re.IGNORECASE)
                
            # Clean up multiple punctuation
            text = re.sub(r'!+', '!', text)
            text = re.sub(r'\?+', '?', text)
            text = re.sub(r'\.+', '.', text)
            
            # Clean up spaces
            text = re.sub(r'\s+', ' ', text).strip()
            
        elif style == EnhancementStyle.CONCISE:
            # Remove filler words
            fillers = [
                r'\blike\b', r'\bjust\b', r'\breally\b', r'\bvery\b',
                r'\bactually\b', r'\bbasically\b', r'\bliterally\b',
                r'\bkind of\b', r'\bsort of\b'
            ]
            for filler in fillers:
                text = re.sub(filler, '', text, flags=re.IGNORECASE)
            text = re.sub(r'\s+', ' ', text).strip()
        
        return text
    
    def _rule_based_fallback(self, message: str, style: EnhancementStyle, tone: EnhancementTone) -> str:
        """
        Pure rule-based enhancement as fallback when LLM fails.
        """
        text = self._rule_based_preprocess(message, style)
        
        # Capitalize first letter
        if text and text[0].islower():
            text = text[0].upper() + text[1:]
            
        # Add appropriate punctuation
        if text and not text[-1] in ".!?":
            if style == EnhancementStyle.FRIENDLY or tone == EnhancementTone.ENTHUSIASTIC:
                text += "!"
            else:
                text += "."
                
        # Tone adjustments
        if tone == EnhancementTone.CONFIDENT:
            # Remove hedging
            hedges = [r'\bmaybe\b', r'\bperhaps\b', r'\bpossibly\b', r'\bI think\b', r'\bI guess\b']
            for hedge in hedges:
                text = re.sub(hedge, '', text, flags=re.IGNORECASE)
            text = re.sub(r'\s+', ' ', text).strip()
        
        return text
    
    async def enhance(
        self,
        message: str,
        style: EnhancementStyle = EnhancementStyle.PROFESSIONAL,
        tone: EnhancementTone = EnhancementTone.NEUTRAL
    ) -> tuple[str, dict]:
        """
        Enhance a text message.
        
        Args:
            message: The original message
            style: Enhancement style
            tone: Enhancement tone
            
        Returns:
            Tuple of (enhanced_message, metadata)
            metadata contains: method (llm/fallback), processing_time, etc.
        """
        import time
        start_time = time.time()
        
        # Preprocess with rules
        preprocessed = self._rule_based_preprocess(message, style)
        
        metadata = {
            "method": "llm",
            "model": self.ollama.model,
            "style": style.value,
            "tone": tone.value,
        }
        
        try:
            system_prompt = self._build_system_prompt(style, tone)
            user_prompt = f"Enhance this message:\n\n{preprocessed}"
            
            response = await self.ollama.chat(
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_prompt}
                ],
                temperature=0.7,
                max_tokens=min(len(message) * 3, 500),
            )
            
            enhanced = response.content.strip()
            
            # Clean up any meta-text the model might have added
            enhanced = re.sub(r'^(Here\'s|Here is).*?:', '', enhanced, flags=re.IGNORECASE).strip()
            enhanced = enhanced.strip('"').strip("'")
            
            # Validate output
            if not enhanced or len(enhanced) < 3:
                raise ValueError("LLM output too short")
            
            # Don't allow output that's way too long
            if len(enhanced) > len(message) * 4:
                raise ValueError("LLM output too long")
            
            metadata["llm_duration_ns"] = response.total_duration
            metadata["tokens_generated"] = response.eval_count
            
            processing_time = (time.time() - start_time) * 1000
            metadata["processing_time_ms"] = round(processing_time, 2)
            
            return enhanced, metadata
        except Exception as e:
            logger.warning(f"LLM enhancement failed, using rule-based fallback: {e}")
            
            if not self.enable_fallback:
                raise
            
            # Use rule-based fallback
            enhanced = self._rule_based_fallback(message, style, tone)
            metadata["method"] = "fallback"
            metadata["error"] = str(e)
            
            processing_time = (time.time() - start_time) * 1000
            metadata["processing_time_ms"] = round(processing_time, 2)
            
            return enhanced, metadata
    
    def analyze_improvements(self, original: str, enhanced: str, style: str, tone: str) -> list[str]:
        """
        Analyze what improvements were made.
        
        Returns:
            List of improvement descriptions
        """
        improvements = []
        
        # Length changes
        if len(enhanced) > len(original) * 1.2:
            improvements.append("Expanded with more detail")
        elif len(enhanced) < len(original) * 0.8:
            improvements.append("Made more concise")
        
        # Capitalization
        if enhanced[0].isupper() and not original[0].isupper():
            improvements.append("Capitalized first letter")
        
        # Punctuation
        if enhanced[-1] in '.!?' and not original[-1] in '.!?':
            improvements.append("Added proper punctuation")
        
        # Informal language
        informal_words = ['u', 'ur', 'plz', 'thx', 'bro', 'dude', 'gonna', 'wanna']
        had_informal = any(re.search(rf'\b{word}\b', original.lower()) for word in informal_words)
        has_informal = any(re.search(rf'\b{word}\b', enhanced.lower()) for word in informal_words)
        
        if had_informal and not has_informal:
            improvements.append("Replaced informal language")
        
        # Grammar
        if '  ' in original and '  ' not in enhanced:
            improvements.append("Fixed spacing")
        
        if not improvements:
            improvements.append(f"Applied {style} style")
            improvements.append(f"Used {tone} tone")
        
        return improvements