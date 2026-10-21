import pytest

from visper_ai.api import dependencies
from visper_ai.core.ai.enhancement_service import USER_MESSAGE_CLOSE, USER_MESSAGE_OPEN

from .conftest import OTHER_USER_ID, USER_ID, seed_user

ENHANCE_URL = "/api/v1/ai/enhance"


class BrokenRedis:
    async def ping(self):
        raise ConnectionError("redis down")


async def test_health(client):
    response = await client.get("/api/v1/health")
    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "healthy"
    assert body["version"]  # APP_VERSION used to default to None
    assert response.headers["cache-control"] == "no-store"
    assert "x-request-id" in response.headers


async def test_ready_ok(client):
    response = await client.get("/api/v1/ready")
    assert response.status_code == 200
    assert response.json()["redis"] == "healthy"


async def test_ready_redis_down(app, client):
    from visper_ai.core.utils.cache import async_get_redis

    async def _broken():
        yield BrokenRedis()

    app.dependency_overrides[async_get_redis] = _broken
    response = await client.get("/api/v1/ready")
    assert response.status_code == 503
    assert response.json()["status"] == "unhealthy"


@pytest.mark.parametrize("healthy, expected", [(True, "healthy"), (False, "unhealthy")])
async def test_ai_health_contract(client, fake_ollama, healthy, expected):
    # The Go SDK reads `status == "healthy"` from a 200 response.
    fake_ollama.healthy = healthy
    response = await client.get("/api/v1/ai/health")
    assert response.status_code == 200
    assert response.json()["status"] == expected


async def test_enhance_requires_user(client):
    response = await client.post(ENHANCE_URL, json={"message": "hello there"})
    assert response.status_code == 401


async def test_enhance_unknown_user(client):
    response = await client.post(ENHANCE_URL, json={"message": "hello there"}, headers={"X-User-ID": "nope"})
    assert response.status_code == 401


async def test_enhance_success_and_cache(client, fake_redis, fake_ollama):
    await seed_user(fake_redis)
    headers = {"X-User-ID": USER_ID}
    payload = {"message": "  u send me the report plz  ", "style": "professional", "tone": "polite"}

    first = await client.post(ENHANCE_URL, json=payload, headers=headers)
    assert first.status_code == 200, first.text
    body = first.json()
    # Fields consumed by the Go SDK (AIEnhanceResponse)
    assert body["original"] == "u send me the report plz"
    assert body["enhanced"] == fake_ollama.reply
    assert body["style"] == "professional"
    assert body["tone"] == "polite"
    assert isinstance(body["improvements"], list)
    assert body["cached"] is False
    assert body["metadata"]["method"] == "llm"

    # The user text is delimited inside the prompt.
    user_prompt = fake_ollama.calls[0]["messages"][1]["content"]
    assert USER_MESSAGE_OPEN in user_prompt and USER_MESSAGE_CLOSE in user_prompt

    second = await client.post(ENHANCE_URL, json=payload, headers=headers)
    assert second.status_code == 200
    assert second.json()["cached"] is True
    assert len(fake_ollama.calls) == 1


async def test_enhance_fallback_is_not_cached(client, fake_redis, fake_ollama):
    await seed_user(fake_redis)
    fake_ollama.error = RuntimeError("Cannot connect to Ollama")
    headers = {"X-User-ID": USER_ID}
    payload = {"message": "thx bro", "style": "professional"}

    first = await client.post(ENHANCE_URL, json=payload, headers=headers)
    assert first.status_code == 200
    assert first.json()["metadata"]["method"] == "fallback"
    assert first.json()["enhanced"]  # never empty, even when rules strip words

    fake_ollama.error = None
    fake_ollama.reply = "Thank you."
    second = await client.post(ENHANCE_URL, json=payload, headers=headers)
    assert second.json()["cached"] is False
    assert second.json()["metadata"]["method"] == "llm"


@pytest.mark.parametrize("message", ["  a  ", "x" * 2001])
async def test_enhance_validation(client, fake_redis, message):
    await seed_user(fake_redis)
    response = await client.post(ENHANCE_URL, json={"message": message}, headers={"X-User-ID": USER_ID})
    assert response.status_code == 422


async def test_enhance_rate_limited(client, fake_redis, monkeypatch):
    monkeypatch.setattr(dependencies, "DEFAULT_LIMIT", 2)
    await seed_user(fake_redis, is_guest=False)
    headers = {"X-User-ID": USER_ID}

    codes = []
    for i in range(3):
        response = await client.post(ENHANCE_URL, json={"message": f"message number {i}"}, headers=headers)
        codes.append(response.status_code)
    assert codes == [200, 200, 429]


async def test_batch_rejects_oversized_item(client, fake_redis):
    await seed_user(fake_redis)
    response = await client.post(
        "/api/v1/ai/enhance/batch",
        json={"messages": ["fine message", "x" * 5000]},
        headers={"X-User-ID": USER_ID},
    )
    assert response.status_code == 422


async def test_async_task_is_owner_only(client, fake_redis):
    await seed_user(fake_redis, USER_ID)
    await seed_user(fake_redis, OTHER_USER_ID)

    created = await client.post(
        "/api/v1/ai/enhance/async", json={"message": "hello there"}, headers={"X-User-ID": USER_ID}
    )
    assert created.status_code == 202
    task_id = created.json()["task_id"]

    # Background tasks complete before the ASGI call returns.
    mine = await client.get(f"/api/v1/ai/enhance/status/{task_id}", headers={"X-User-ID": USER_ID})
    assert mine.status_code == 200
    assert mine.json()["status"] == "completed"
    assert mine.headers["cache-control"] == "no-store"

    theirs = await client.get(f"/api/v1/ai/enhance/status/{task_id}", headers={"X-User-ID": OTHER_USER_ID})
    assert theirs.status_code == 404


async def test_openapi_renders(client):
    response = await client.get("/openapi.json")
    assert response.status_code == 200
    assert "/api/v1/ai/enhance" in response.json()["paths"]
