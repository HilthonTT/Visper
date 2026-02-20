import os
from enum import Enum
from pydantic import computed_field
from pydantic_settings import BaseSettings, SettingsConfigDict

class AppSettings(BaseSettings):
    APP_NAME: str = "AI FastAPI App"
    APP_DESCRIPTION: str | None = None
    APP_VERSION: str | None = None
    LICENSE_NAME: str | None = None
    
class FileLoggerSettings(BaseSettings):
    FILE_LOG_MAX_BYTES: int = 10 * 1024 * 1024
    FILE_LOG_BACKUP_COUNT: int = 5
    FILE_LOG_FORMAT_JSON: bool = True
    FILE_LOG_LEVEL: str = "INFO"

    # Include request ID, path, method, client host, and status code in the file log
    FILE_LOG_INCLUDE_REQUEST_ID: bool = True
    FILE_LOG_INCLUDE_PATH: bool = True
    FILE_LOG_INCLUDE_METHOD: bool = True
    FILE_LOG_INCLUDE_CLIENT_HOST: bool = True
    FILE_LOG_INCLUDE_STATUS_CODE: bool = True


class ConsoleLoggerSettings(BaseSettings):
    CONSOLE_LOG_LEVEL: str = "INFO"
    CONSOLE_LOG_FORMAT_JSON: bool = False

    # Include request ID, path, method, client host, and status code in the console log
    CONSOLE_LOG_INCLUDE_REQUEST_ID: bool = False
    CONSOLE_LOG_INCLUDE_PATH: bool = False
    CONSOLE_LOG_INCLUDE_METHOD: bool = False
    CONSOLE_LOG_INCLUDE_CLIENT_HOST: bool = False
    CONSOLE_LOG_INCLUDE_STATUS_CODE: bool = False
    
class RedisCacheSettings(BaseSettings):
    # The GO Backend prefix
    REDIS_KEY_PREFIX: str = "visper:"
    
    REDIS_CACHE_HOST: str = "localhost"
    REDIS_CACHE_PORT: int = 6379
    REDIS_CACHE_PASSWORD: str | None = None 

    @computed_field  # type: ignore[prop-decorator]
    @property
    def REDIS_CACHE_URL(self) -> str:
        if self.REDIS_CACHE_PASSWORD:
            return f"redis://:{self.REDIS_CACHE_PASSWORD}@{self.REDIS_CACHE_HOST}:{self.REDIS_CACHE_PORT}"
        return f"redis://{self.REDIS_CACHE_HOST}:{self.REDIS_CACHE_PORT}"

class ClientSideCacheSettings(BaseSettings):
    CLIENT_CACHE_MAX_AGE: int = 60
    
class RedisQueueSettings(BaseSettings):
    REDIS_QUEUE_HOST: str = "localhost"
    REDIS_QUEUE_PORT: int = 6379
    REDIS_QUEUE_PASSWORD: str | None = None 

class RedisRateLimiterSettings(BaseSettings):
    REDIS_RATE_LIMIT_HOST: str = "localhost"
    REDIS_RATE_LIMIT_PORT: int = 6379
    REDIS_RATE_LIMIT_PASSWORD: str | None = None

    @computed_field  # type: ignore[prop-decorator]
    @property
    def REDIS_RATE_LIMIT_URL(self) -> str:
        if self.REDIS_RATE_LIMIT_PASSWORD:
            return f"redis://:{self.REDIS_RATE_LIMIT_PASSWORD}@{self.REDIS_RATE_LIMIT_HOST}:{self.REDIS_RATE_LIMIT_PORT}"
        return f"redis://{self.REDIS_RATE_LIMIT_HOST}:{self.REDIS_RATE_LIMIT_PORT}"
    
class DefaultRateLimitSettings(BaseSettings):
    DEFAULT_RATE_LIMIT_LIMIT: int = 10
    DEFAULT_RATE_LIMIT_PERIOD: int = 3600

class EnvironmentOption(str, Enum):
    LOCAL = "local"
    STAGING = "staging"
    PRODUCTION = "production"

class EnvironmentSettings(BaseSettings):
    ENVIRONMENT: EnvironmentOption = EnvironmentOption.LOCAL


class CORSSettings(BaseSettings):
    CORS_ORIGINS: list[str] = ["*"]
    CORS_METHODS: list[str] = ["*"]
    CORS_HEADERS: list[str] = ["*"]
    
class OllamaSettings(BaseSettings):
    """Ollama LLM settings"""
    OLLAMA_HOST: str = "localhost"
    OLLAMA_PORT: int = 11434
    OLLAMA_MODEL: str = "qwen2.5:3b"  # Lightweight 3B model, great for text enhancement
    OLLAMA_TIMEOUT: int = 120
    OLLAMA_MAX_TOKENS: int = 500
    OLLAMA_TEMPERATURE: float = 0.7
    
    @computed_field  # type: ignore[prop-decorator]
    @property
    def OLLAMA_BASE_URL(self) -> str:
        return f"http://{self.OLLAMA_HOST}:{self.OLLAMA_PORT}"

class AIEnhancementSettings(BaseSettings):
    """AI text enhancement settings"""
    # Cache settings
    ENHANCEMENT_CACHE_TTL: int = 3600  # 1 hour
    
    # Processing settings
    MAX_MESSAGE_LENGTH: int = 2000
    MIN_MESSAGE_LENGTH: int = 3
    BATCH_SIZE_LIMIT: int = 5
    
    # Fallback settings
    ENABLE_RULE_BASED_FALLBACK: bool = True
    
class Settings(
    AppSettings,
    RedisCacheSettings,
    ClientSideCacheSettings,
    RedisQueueSettings,
    RedisRateLimiterSettings,
    DefaultRateLimitSettings,
    EnvironmentSettings,
    CORSSettings,
    FileLoggerSettings,
    ConsoleLoggerSettings,
    OllamaSettings,
    AIEnhancementSettings,
):
    model_config = SettingsConfigDict(
        env_file=os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "..", ".env"),
        env_file_encoding="utf-8",
        case_sensitive=True,
        extra="ignore",
    )

settings = Settings()
