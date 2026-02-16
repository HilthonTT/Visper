from pydantic import BaseModel, Field, field_validator
from ..core.ai.enhancement_service import EnhancementStyle, EnhancementTone

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


class AsyncEnhanceRequest(BaseModel):
    """Request to enhance a message asynchronously"""
    message: str = Field(..., min_length=3, max_length=2000)
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL
    callback_url: str | None = None  # Optional webhook for completion
    
    @field_validator('message')
    @classmethod
    def validate_message(cls, v: str) -> str:
        v = v.strip()
        if not v:
            raise ValueError("Message cannot be empty")
        return v


class AsyncEnhanceResponse(BaseModel):
    """Immediate response for async request"""
    task_id: str
    status: str = "pending"
    message: str
    estimated_time_seconds: int = 2


class TaskStatusResponse(BaseModel):
    """Status of an async enhancement task"""
    task_id: str
    status: str  # pending, processing, completed, failed
    created_at: str
    completed_at: str | None = None
    
    # Only present when completed
    original: str | None = None
    enhanced: str | None = None
    style: str | None = None
    tone: str | None = None
    improvements: list[str] | None = None
    metadata: dict | None = None
    
    # Only present when failed
    error: str | None = None


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
