from typing import Literal
from pydantic import BaseModel, Field, field_validator

class EnhanceMessageRequest(BaseModel):
    message: str = Field(
        ...,
        min_length=1,
        max_length=2000,
        description="Original message to enhance"
    )
    style: Literal["professional", "casual", "concise", "detailed", "friendly"] = Field(
        default="professional",
        description="Enhancement style"
    )
    tone: Literal["neutral", "confident", "empathetic", "persuasive"] = Field(
        default="neutral",
        description="Desired tone"
    )
    preserve_intent: bool = Field(
        default=True,
        description="Preserve original message intent"
    )
    
    @field_validator("message")
    def validate_message(cls, v):
        """Validate and sanitize message"""
        stripped = v.strip()
        if not stripped:
            raise ValueError("Message cannot be empty or whitespace only")
        if len(stripped) < 5:
            raise ValueError("Message too short to enhance (minimum 5 characters)")
        return stripped
    
    class Config:
        json_schema_extra = {
            "example": {
                "message": "hey can u help me with the project deadline?",
                "style": "professional",
                "tone": "confident",
                "preserve_intent": True
            }
        }

class EnhanceMessageResponse(BaseModel):
    original: str
    enhanced: str
    style: str
    tone: str
    improvements: list[str]
    processing_time_ms: float
    cached: bool = False

class BatchEnhanceRequest(BaseModel):
    messages: list[str] = Field(..., min_items=1, max_items=5)
    style: str = "professional"
    tone: str = "neutral"
