import json
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass, field

# Must be set before visper_ai is imported (settings are read at import time).
os.environ["FILE_LOG_ENABLED"] = "false"
os.environ["ENVIRONMENT"] = "local"

import fakeredis
import httpx
import pytest
from fastapi import FastAPI

from visper_ai.api import router
from visper_ai.api.v1 import ai_bot
from visper_ai.core.ai.enhancement_service import TextEnhancementService
from visper_ai.core.ai.ollama_client import OllamaResponse
from visper_ai.core.config import settings
from visper_ai.core.setup import create_application
from visper_ai.core.utils.cache import async_get_redis
from visper_ai.core.utils.rate_limit import async_get_rate_limit_redis

USER_ID = "11111111-1111-1111-1111-111111111111"
OTHER_USER_ID = "22222222-2222-2222-2222-222222222222"


@dataclass
class FakeOllama:
    """Stand-in for OllamaClient: records calls, returns a canned reply or raises."""

    model: str = "test-model"
    max_tokens: int = 500
    reply: str = "Could you please send me the report?"
    error: Exception | None = None
    healthy: bool = True
    calls: list[dict] = field(default_factory=list)

    async def chat(self, messages, temperature=None, max_tokens=None) -> OllamaResponse:
        self.calls.append({"messages": messages, "temperature": temperature, "max_tokens": max_tokens})
        if self.error is not None:
            raise self.error
        return OllamaResponse(content=self.reply, model=self.model, total_duration=1, eval_count=2)

    async def health_check(self) -> bool:
        return self.healthy

    async def list_models(self) -> list[str]:
        return [self.model] if self.healthy else []

    async def close(self) -> None:
        pass


@pytest.fixture
async def fake_redis() -> AsyncIterator[fakeredis.FakeAsyncRedis]:
    redis = fakeredis.FakeAsyncRedis()
    yield redis
    await redis.aclose()


@pytest.fixture
def fake_ollama(monkeypatch) -> FakeOllama:
    ollama = FakeOllama()
    service = TextEnhancementService(ollama_client=ollama, enable_fallback=True)  # type: ignore[arg-type]
    monkeypatch.setattr(ai_bot, "_enhancement_service", service)
    return ollama


@asynccontextmanager
async def _noop_lifespan(app: FastAPI):
    yield


@pytest.fixture
def app(fake_redis, fake_ollama) -> FastAPI:
    application = create_application(router=router, settings=settings, lifespan=_noop_lifespan)

    async def _redis():
        yield fake_redis

    application.dependency_overrides[async_get_redis] = _redis
    application.dependency_overrides[async_get_rate_limit_redis] = _redis
    return application


@pytest.fixture
async def client(app) -> AsyncIterator[httpx.AsyncClient]:
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as c:
        yield c


async def seed_user(redis, user_id: str = USER_ID, *, is_guest: bool = False) -> None:
    """Store a user exactly like the Go API does (JSON under `visper:user:<id>`)."""
    payload = {"id": user_id, "username": "alice", "isGuest": is_guest, "created_at": "2026-01-01T00:00:00Z"}
    await redis.set(f"{settings.REDIS_KEY_PREFIX}user:{user_id}", json.dumps(payload))
