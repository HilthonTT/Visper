from typing import Annotated

from pydantic import AnyHttpUrl, BaseModel, Field, StringConstraints, field_validator

from ..core.ai.enhancement_service import EnhancementStyle, EnhancementTone
from ..core.config import settings

# Whitespace is stripped *before* the length checks (a field_validator running after
# Field(min_length=3) let "  a  " through as "a").
MessageText = Annotated[
    str,
    StringConstraints(
        strip_whitespace=True,
        min_length=settings.MIN_MESSAGE_LENGTH,
        max_length=settings.MAX_MESSAGE_LENGTH,
    ),
]
BatchMessageText = Annotated[str, StringConstraints(strip_whitespace=True, max_length=settings.MAX_MESSAGE_LENGTH)]


class EnhanceRequest(BaseModel):
    """Request to enhance a single message"""

    message: MessageText
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL


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

    message: MessageText
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL
    callback_url: AnyHttpUrl | None = None  # Optional webhook for completion (http/https only)


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

    # Each item is length-capped too: previously only the list size was bounded, so a
    # single batch item could be arbitrarily large.
    messages: list[BatchMessageText] = Field(..., min_length=1, max_length=settings.BATCH_SIZE_LIMIT)
    style: EnhancementStyle = EnhancementStyle.PROFESSIONAL
    tone: EnhancementTone = EnhancementTone.NEUTRAL

    @field_validator("messages")
    @classmethod
    def validate_messages(cls, v: list[str]) -> list[str]:
        messages = [msg for msg in v if msg]
        if not messages:
            raise ValueError("At least one non-empty message required")
        return messages


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
