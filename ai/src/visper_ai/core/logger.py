"""Logging configuration for the application."""

import logging
from logging.handlers import RotatingFileHandler
from pathlib import Path

import structlog
from structlog.dev import ConsoleRenderer
from structlog.processors import JSONRenderer
from structlog.types import EventDict, Processor

from .config import settings


def drop_color_message_key(_, __, event_dict: EventDict) -> EventDict:
    """Uvicorn adds `color_message` which duplicates `event`.

    Remove it to avoid double logging.
    """
    event_dict.pop("color_message", None)
    return event_dict


def file_log_filter_processors(_, __, event_dict: EventDict) -> EventDict:
    """Filter out the request ID, path, method, client host, and status code from the event dict if the
    corresponding setting is False."""

    if not settings.FILE_LOG_INCLUDE_REQUEST_ID:
        event_dict.pop("request_id", None)
    if not settings.FILE_LOG_INCLUDE_PATH:
        event_dict.pop("path", None)
    if not settings.FILE_LOG_INCLUDE_METHOD:
        event_dict.pop("method", None)
    if not settings.FILE_LOG_INCLUDE_CLIENT_HOST:
        event_dict.pop("client_host", None)
    if not settings.FILE_LOG_INCLUDE_STATUS_CODE:
        event_dict.pop("status_code", None)
    return event_dict


def console_log_filter_processors(_, __, event_dict: EventDict) -> EventDict:
    """Filter out the request ID, path, method, client host, and status code from the event dict if the
    corresponding setting is False."""

    if not settings.CONSOLE_LOG_INCLUDE_REQUEST_ID:
        event_dict.pop("request_id", None)
    if not settings.CONSOLE_LOG_INCLUDE_PATH:
        event_dict.pop("path", None)
    if not settings.CONSOLE_LOG_INCLUDE_METHOD:
        event_dict.pop("method", None)
    if not settings.CONSOLE_LOG_INCLUDE_CLIENT_HOST:
        event_dict.pop("client_host", None)
    if not settings.CONSOLE_LOG_INCLUDE_STATUS_CODE:
        event_dict.pop("status_code", None)
    return event_dict


# Shared processors for all loggers
timestamper = structlog.processors.TimeStamper(fmt="iso")
SHARED_PROCESSORS: list[Processor] = [
    structlog.contextvars.merge_contextvars,
    structlog.stdlib.add_logger_name,
    structlog.stdlib.add_log_level,
    structlog.stdlib.PositionalArgumentsFormatter(),
    structlog.stdlib.ExtraAdder(),
    drop_color_message_key,
    timestamper,
    structlog.processors.StackInfoRenderer(),
]


def build_formatter(*, json_output: bool, pre_chain: list[Processor]) -> structlog.stdlib.ProcessorFormatter:
    """Build a ProcessorFormatter with the specified renderer and processors."""
    renderer = JSONRenderer() if json_output else ConsoleRenderer()

    processors = [structlog.stdlib.ProcessorFormatter.remove_processors_meta, renderer]

    if json_output:
        pre_chain = pre_chain + [structlog.processors.format_exc_info]

    return structlog.stdlib.ProcessorFormatter(foreign_pre_chain=pre_chain, processors=processors)


_configured = False


def setup_logging() -> None:
    """Configure structlog + stdlib logging once (idempotent).

    Called explicitly by the API and worker entrypoints instead of running as an
    import side effect, so importing the package never creates files.
    """
    global _configured
    if _configured:
        return

    structlog.configure(
        processors=SHARED_PROCESSORS + [structlog.stdlib.ProcessorFormatter.wrap_for_formatter],
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )

    handlers: list[logging.Handler] = []

    console_handler = logging.StreamHandler()
    console_handler.setLevel(settings.CONSOLE_LOG_LEVEL)
    console_handler.setFormatter(
        build_formatter(
            json_output=settings.CONSOLE_LOG_FORMAT_JSON,
            pre_chain=SHARED_PROCESSORS + [console_log_filter_processors],
        )
    )
    handlers.append(console_handler)

    if settings.FILE_LOG_ENABLED:
        log_dir = Path(settings.LOG_DIR).resolve()
        log_dir.mkdir(parents=True, exist_ok=True)
        file_handler = RotatingFileHandler(
            filename=log_dir / "app.log",
            maxBytes=settings.FILE_LOG_MAX_BYTES,
            backupCount=settings.FILE_LOG_BACKUP_COUNT,
            encoding="utf-8",
        )
        file_handler.setLevel(settings.FILE_LOG_LEVEL)
        file_handler.setFormatter(
            build_formatter(
                json_output=settings.FILE_LOG_FORMAT_JSON,
                pre_chain=SHARED_PROCESSORS + [file_log_filter_processors],
            )
        )
        handlers.append(file_handler)

    # The root level must not be stricter than the most verbose handler, otherwise
    # e.g. CONSOLE_LOG_LEVEL=DEBUG would silently never emit anything.
    root_logger = logging.getLogger()
    root_logger.handlers.clear()  # avoid duplicate logs
    root_logger.setLevel(min(h.level for h in handlers))
    for handler in handlers:
        root_logger.addHandler(handler)

    # Uvicorn logger integration
    for logger_name in ("uvicorn", "uvicorn.error", "uvicorn.access"):
        uvicorn_logger = logging.getLogger(logger_name)
        uvicorn_logger.handlers.clear()
        uvicorn_logger.propagate = True

    _configured = True
