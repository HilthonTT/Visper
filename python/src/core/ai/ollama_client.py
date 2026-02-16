import httpx
import logging
from typing import Optional, AsyncIterator
from dataclasses import dataclass

logger = logging.getLogger(__name__)

@dataclass
class OllamaResponse:
    """Ollama API response"""
    content: str
    model: str
    total_duration: Optional[int] = None
    prompt_eval_count: Optional[int] = None
    eval_count: Optional[int] = None
    
class OllamaClient:
    """
    Async client for Ollama API.
    
    Ollama is a local LLM server that's much more efficient than loading
    transformers models directly into Python.
    """
    
    def __init__(self, base_url: str, model: str, timeout: int = 30, max_tokens: int = 500, temperature: float = 0.7):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.timeout = timeout
        self.max_tokens = max_tokens
        self.temperature = temperature
        self._client = httpx.AsyncClient(timeout=timeout)
        
    async def __aenter__(self):
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
    
    async def close(self):
        """Close the HTTP client"""
        await self._client.aclose()
        
    async def health_check(self) -> bool:
        """Check if Ollama server is healthy"""
        try:
            response = await self._client.get(f"{self.base_url}/api/tags")
            return response.status_code == 200
        except Exception as e:
            logger.error(f"Ollama health check failed: {e}")
            return False
        
    async def list_models(self) -> list[str]:
        """List available models"""
        try:
            response = await self._client.get(f"{self.base_url}/api/tags")
            response.raise_for_status()
            data = response.json()
            return [model["name"] for model in data.get("models", [])]
        except Exception as e:
            logger.error(f"Failed to list models: {e}")
            return []
        
    async def generate(
        self, 
        prompt: str, 
        system: Optional[str] = None,
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None
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
        payload = {
            "model": self.model,
            "prompt": prompt,
            "stream": False,
            "options": {
                "temperature": temperature or self.temperature,
                "num_predict": max_tokens or self.max_tokens,
            }
        }
        
        if system:
            payload["system"] = system
            
        try:
            response = await self._client.post(f"{self.base_url}/api/generate", json=payload)
            response.raise_for_status()
            data = response.json()
            
            return OllamaResponse(
                content=data["response"].strip(),
                model=data.get("model", self.model),
                total_duration=data.get("total_duration"),
                prompt_eval_count=data.get("prompt_eval_count"),
                eval_count=data.get("eval_count"),
            )
        except httpx.HTTPError as e:
            logger.error(f"Ollama HTTP error: {e}")
            raise RuntimeError(f"Failed to generate completion: {e}")
        except Exception as e:
            logger.error(f"Ollama generation error: {e}")
            raise RuntimeError(f"Unexpected error during generation: {e}")
            
    async def chat(
        self,
        messages: list[dict[str, str]],
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None,
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
            "options": {
                "temperature": temperature or self.temperature,
                "num_predict": max_tokens or self.max_tokens,
            }
        }
        
        try:
            response = await self._client.post(
                f"{self.base_url}/api/chat",
                json=payload
            )
            response.raise_for_status()
            data = response.json()
            
            return OllamaResponse(
                content=data["message"]["content"].strip(),
                model=data.get("model", self.model),
                total_duration=data.get("total_duration"),
                prompt_eval_count=data.get("prompt_eval_count"),
                eval_count=data.get("eval_count"),
            )
            
        except httpx.HTTPError as e:
            logger.error(f"Ollama HTTP error: {e}")
            raise RuntimeError(f"Failed to chat: {e}")
        except Exception as e:
            logger.error(f"Ollama chat error: {e}")
            raise RuntimeError(f"Unexpected error during chat: {e}")
    
    async def stream_generate(
        self,
        prompt: str,
        system: Optional[str] = None,
        temperature: Optional[float] = None,
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
        payload = {
            "model": self.model,
            "prompt": prompt,
            "stream": True,
            "options": {
                "temperature": temperature or self.temperature,
            }
        }
        
        if system:
            payload["system"] = system
        
        try:
            async with self._client.stream(
                "POST",
                f"{self.base_url}/api/generate",
                json=payload
            ) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if line:
                        import json
                        data = json.loads(line)
                        if "response" in data:
                            yield data["response"]
                        if data.get("done", False):
                            break
                            
        except Exception as e:
            logger.error(f"Ollama streaming error: {e}")
            raise RuntimeError(f"Failed to stream generation: {e}")