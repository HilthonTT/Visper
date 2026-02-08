from fastapi import APIRouter

from .health import router as health_router
from .ai_bot import router as ai_bot_router

router = APIRouter(prefix="/v1")
router.include_router(health_router)
router.include_router(ai_bot_router)
