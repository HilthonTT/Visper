import re
import time
import uuid
from collections.abc import Awaitable, Callable
from typing import Any

import structlog
from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.responses import Response
from starlette.types import ASGIApp

# uuid.uuid7() only exists on Python >= 3.14; fall back to uuid4 on 3.12/3.13
# (previously every request raised AttributeError there).
_new_request_id: Callable[[], uuid.UUID] = getattr(uuid, "uuid7", uuid.uuid4)

# Client-supplied request IDs are logged and echoed back: keep them short and safe.
_REQUEST_ID_RE = re.compile(r"^[A-Za-z0-9._-]{1,128}$")


def resolve_request_id(header_value: str | None) -> str:
    if header_value and _REQUEST_ID_RE.fullmatch(header_value):
        return header_value
    return str(_new_request_id())


class LoggerMiddleware(BaseHTTPMiddleware):
    """Middleware to add request context and logging metadata.

    This middleware automatically:
    - Generates or extracts request IDs
    - Binds request metadata to structlog context
    - Adds request ID to response headers
    - Tracks request duration
    - Logs request completion

    Parameters
    ----------
    app : FastAPI
        The FastAPI application instance.
    dispatch : callable, optional
        Custom dispatch function, by default None.
    log_request_body : bool, optional
        Whether to log request body size, by default False.
    exclude_paths : set of str, optional
        Paths to exclude from logging (e.g., health checks), by default None.

    Attributes
    ----------
    log_request_body : bool
        Flag indicating if request body should be logged.
    exclude_paths : set of str
        Set of paths to exclude from detailed logging.
    logger : structlog.BoundLogger
        Structured logger instance.

    Notes
    -----
    - Uses UUID7 (UUID4 before Python 3.14) for request IDs
    - Automatically clears context between requests to prevent context leakage
    - Adds X-Request-ID header to responses for request tracing
    """

    def __init__(
        self,
        app: ASGIApp,
        dispatch: Callable[[Request, RequestResponseEndpoint], Awaitable[Response]] | None = None,
        log_request_body: bool = False,
        exclude_paths: set[str] | None = None,
    ) -> None:
        """Initialize the LoggerMiddleware.

        Parameters
        ----------
        app : FastAPI
            The FastAPI application instance.
        dispatch : callable, optional
            Custom dispatch function, by default None.
        log_request_body : bool, optional
            Whether to log request body size, by default False.
        exclude_paths : set of str, optional
            Paths to exclude from logging, by default None.
        """
        super().__init__(app, dispatch)
        self.log_request_body = log_request_body
        self.exclude_paths = exclude_paths or set()
        self.logger = structlog.get_logger(__name__)

    def _should_log(self, path: str) -> bool:
        """Determine if the request should be logged.

        Parameters
        ----------
        path : str
            The request path.

        Returns
        -------
        bool
            True if the request should be logged, False otherwise.
        """
        return path not in self.exclude_paths

    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        """Process the request and bind context variables for logging.

        This method:
        1. Generates or extracts a request ID
        2. Clears previous context variables
        3. Binds request metadata to logging context
        4. Processes the request
        5. Updates context with response metadata
        6. Logs the completed request
        7. Returns response with X-Request-ID header

        Parameters
        ----------
        request : Request
            The incoming HTTP request.
        call_next : RequestResponseEndpoint
            The next middleware or route handler in the chain.

        Returns
        -------
        Response
            The HTTP response with added request ID header.

        Notes
        -----
        Context variables are cleared at the start to prevent leakage between requests.
        """
        # Generate or extract request ID
        request_id = resolve_request_id(request.headers.get("X-Request-ID"))

        # Clear context to prevent leakage from previous requests
        structlog.contextvars.clear_contextvars()

        # Start timing
        start_time = time.perf_counter()

        # Build initial context
        context: dict[str, Any] = {
            "request_id": request_id,
            "path": request.url.path,
            "method": request.method,
            "client_host": request.client.host if request.client else None,
            "user_agent": request.headers.get("user-agent"),
        }

        # Optionally add request body size
        if self.log_request_body:
            content_length = request.headers.get("content-length")
            if content_length:
                context["request_body_size"] = int(content_length)

        # Bind initial context
        structlog.contextvars.bind_contextvars(**context)

        # Process request
        try:
            response = await call_next(request)

            duration_ms = (time.perf_counter() - start_time) * 1000

            # Update context with response data
            structlog.contextvars.bind_contextvars(
                status_code=response.status_code,
                duration_ms=round(duration_ms, 2),
            )

            # Log request completion (only if not excluded)
            if self._should_log(request.url.path):
                log_method = (
                    self.logger.error
                    if response.status_code >= 500
                    else self.logger.warning
                    if response.status_code >= 400
                    else self.logger.info
                )
                log_method(
                    "request_completed",
                    status_code=response.status_code,
                    duration_ms=round(duration_ms, 2),
                )

            # Add request ID to response headers
            response.headers["X-Request-ID"] = request_id

            return response

        except Exception as exc:
            # Calculate duration for failed requests
            duration_ms = (time.perf_counter() - start_time) * 1000

            # Log the exception
            structlog.contextvars.bind_contextvars(
                duration_ms=round(duration_ms, 2),
                error=str(exc),
                error_type=type(exc).__name__,
            )

            if self._should_log(request.url.path):
                self.logger.error(
                    "request_failed",
                    exc_info=True,
                    duration_ms=round(duration_ms, 2),
                )

            # Re-raise the exception
            raise
        finally:
            # Context will be automatically cleared on the next request
            pass
