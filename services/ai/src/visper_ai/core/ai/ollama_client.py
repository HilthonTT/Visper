import json
import logging
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

import httpx

logger = logging.getLogger(__name__)

# Health/list calls must answer quickly: a probe that inherits the 120s generation
# timeout makes /health hang for minutes when Ollama is unreachable.
HEALTH_CHECK_TIMEOUT = httpx.Timeout(5.0)


@dataclass
class OllamaResponse:
    """Ollama API response"""

    content: str
    model: str
    total_duration: int | None = None
    prompt_eval_count: int | None = None
    eval_count: int | None = None


class OllamaClient:
    """
    Async client for Ollama API.

    Ollama is a local LLM server that's much more efficient than loading
    transformers models directly into Python.
    """

    def __init__(
        self,
        base_url: str,
        model: str,
        timeout: float = 30,
        max_tokens: int = 500,
        temperature: float = 0.7,
        connect_timeout: float = 5.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.timeout = timeout
        self.max_tokens = max_tokens
        self.temperature = temperature
        # Fail fast on connect, allow the (possibly cold) model time to generate.
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=httpx.Timeout(timeout, connect=connect_timeout),
            transport=transport,
        )

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()

    async def close(self):
        """Close the HTTP client"""
        await self._client.aclose()

    def _options(self, temperature: float | None, max_tokens: int | None) -> dict[str, Any]:
        # `temperature or default` silently replaced an explicit 0.0 with the default.
        return {
            "temperature": self.temperature if temperature is None else temperature,
            "num_predict": self.max_tokens if max_tokens is None else max_tokens,
        }

    async def health_check(self) -> bool:
        """Check if Ollama server is healthy"""
        try:
            response = await self._client.get("/api/tags", timeout=HEALTH_CHECK_TIMEOUT)
            return response.status_code == 200
        except httpx.HTTPError as e:
            logger.error(f"Ollama health check failed: {type(e).__name__}: {e}")
            return False

    async def list_models(self) -> list[str]:
        """List available models"""
        try:
            response = await self._client.get("/api/tags", timeout=HEALTH_CHECK_TIMEOUT)
            response.raise_for_status()
            data = response.json()
            return [model["name"] for model in data.get("models", [])]
        except (httpx.HTTPError, ValueError, KeyError) as e:
            logger.error(f"Failed to list models: {e}")
            return []

    async def generate(
        self,
        prompt: str,
        system: str | None = None,
        temperature: float | None = None,
        max_tokens: int | None = None,
    ) -> OllamaResponse:
        """
        Generate completion from prompt.

        Args:
            prompt: The user prompt
            system: Optional system prompt
            temperature: Sampling temperature (0.0-1.0)
            max_tokens: Maximum tokens to generate

        Returns:
            OllamaResponse with generated content
        """
        payload: dict[str, Any] = {
            "model": self.model,
            "prompt": prompt,
            "stream": False,
            "options": self._options(temperature, max_tokens),
        }

        if system:
            payload["system"] = system

        try:
            response = await self._client.post("/api/generate", json=payload)
            response.raise_for_status()
            data = response.json()

            return OllamaResponse(
                content=data["response"].strip(),
                model=data.get("model", self.model),
                total_duration=data.get("total_duration"),
                prompt_eval_count=data.get("prompt_eval_count"),
                eval_count=data.get("eval_count"),
            )
        except httpx.TimeoutException as e:
            logger.error(f"Ollama timeout after {self.timeout}s: {type(e).__name__}")
            raise RuntimeError(f"Ollama timed out after {self.timeout}s — model may be loading") from e
        except httpx.ConnectError as e:
            logger.error(f"Ollama connection refused at {self.base_url}: {e}")
            raise RuntimeError(f"Cannot connect to Ollama at {self.base_url}") from e
        except httpx.HTTPStatusError as e:
            logger.error(f"Ollama HTTP {e.response.status_code}: {e.response.text}")
            raise RuntimeError(f"Ollama returned {e.response.status_code}") from e
        except httpx.HTTPError as e:
            logger.error(f"Ollama HTTP error ({type(e).__name__}): {e}")
            raise RuntimeError(f"Failed to generate: {e}") from e
        except (ValueError, KeyError) as e:
            logger.error(f"Unexpected Ollama response: {e}")
            raise RuntimeError(f"Unexpected Ollama response: {e}") from e

    async def chat(
        self,
        messages: list[dict[str, str]],
        temperature: float | None = None,
        max_tokens: int | None = None,
    ) -> OllamaResponse:
        """
        Chat completion with message history.

        Args:
            messages: List of message dicts with 'role' and 'content'
                     Role can be 'system', 'user', or 'assistant'
            temperature: Sampling temperature
            max_tokens: Maximum tokens to generate

        Returns:
            OllamaResponse with assistant's reply
        """
        payload = {
            "model": self.model,
            "messages": messages,
            "stream": False,
            "options": self._options(temperature, max_tokens),
        }

        try:
            response = await self._client.post("/api/chat", json=payload)
            response.raise_for_status()
            data = response.json()

            return OllamaResponse(
                content=data["message"]["content"].strip(),
                model=data.get("model", self.model),
                total_duration=data.get("total_duration"),
                prompt_eval_count=data.get("prompt_eval_count"),
                eval_count=data.get("eval_count"),
            )
        except httpx.TimeoutException as e:
            logger.error(f"Ollama timeout after {self.timeout}s: {type(e).__name__}")
            raise RuntimeError(f"Ollama timed out after {self.timeout}s — model may be loading") from e
        except httpx.HTTPError as e:
            logger.error(f"Ollama HTTP error ({type(e).__name__}): {e}")
            raise RuntimeError(f"Failed to chat: {e}") from e
        except (ValueError, KeyError, TypeError) as e:
            logger.error(f"Unexpected Ollama chat response: {e}")
            raise RuntimeError(f"Unexpected Ollama chat response: {e}") from e

    async def stream_generate(
        self,
        prompt: str,
        system: str | None = None,
        temperature: float | None = None,
    ) -> AsyncIterator[str]:
        """
        Stream generation token by token.

        Args:
            prompt: The user prompt
            system: Optional system prompt
            temperature: Sampling temperature

        Yields:
            Generated text chunks
        """
        payload: dict[str, Any] = {
            "model": self.model,
            "prompt": prompt,
            "stream": True,
            "options": {"temperature": self.temperature if temperature is None else temperature},
        }

        if system:
            payload["system"] = system

        try:
            async with self._client.stream("POST", "/api/generate", json=payload) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line:
                        continue
                    data = json.loads(line)
                    if "response" in data:
                        yield data["response"]
                    if data.get("done", False):
                        break
        except (httpx.HTTPError, ValueError) as e:
            logger.error(f"Ollama streaming error: {e}")
            raise RuntimeError(f"Failed to stream generation: {e}") from e
