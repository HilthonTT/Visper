from fastapi import FastAPI, Request, Response
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from typing import Optional, Set

class ClientCacheMiddleware(BaseHTTPMiddleware):
    """Middleware to set the `Cache-Control` header for client-side caching on responses.

    Parameters
    ----------
    app : FastAPI
        The FastAPI application instance.
    max_age : int, optional
        Duration (in seconds) for which the response should be cached. Defaults to 60 seconds.
    exclude_paths : set of str, optional
        Set of paths to exclude from caching (e.g., {"/api/auth", "/api/user"}).
    cacheable_methods : set of str, optional
        HTTP methods that should be cached. Defaults to {"GET", "HEAD"}.
    cacheable_status_codes : set of int, optional
        Status codes that should be cached. Defaults to {200}.

    Attributes
    ----------
    max_age : int
        Duration (in seconds) for which the response should be cached.
    exclude_paths : set of str
        Paths excluded from caching.
    cacheable_methods : set of str
        HTTP methods eligible for caching.
    cacheable_status_codes : set of int
        Status codes eligible for caching.

    Methods
    -------
    async def dispatch(request: Request, call_next: RequestResponseEndpoint) -> Response
        Process the request and conditionally set the `Cache-Control` header.

    Notes
    -----
    - The `Cache-Control` header instructs clients (e.g., browsers) to cache the response.
    - Only safe methods (GET, HEAD) and successful responses are cached by default.
    """
    
    def __init__(
        self,
        app: FastAPI,
        max_age: int = 60,
        exclude_paths: Optional[Set[str]] = None,
        cacheable_methods: Optional[Set[str]] = None,
        cacheable_status_codes: Optional[Set[int]] = None,
    ) -> None:
        """Initialize the ClientCacheMiddleware.

        Parameters
        ----------
        app : FastAPI
            The FastAPI application instance.
        max_age : int, optional
            Cache duration in seconds, by default 60.
        exclude_paths : set of str, optional
            Paths to exclude from caching, by default None.
        cacheable_methods : set of str, optional
            HTTP methods to cache, by default {"GET", "HEAD"}.
        cacheable_status_codes : set of int, optional
            Status codes to cache, by default {200}.
        """
        super().__init__(app)
        
        if max_age < 0:
            raise ValueError("max_age must be non-negative")
        
        self.max_age = max_age
        self.exclude_paths = exclude_paths or set()
        self.cacheable_methods = cacheable_methods or {"GET", "HEAD"}
        self.cacheable_status_codes = cacheable_status_codes or {200}
        
    def _should_cache(self, request: Request, response: Response) -> bool:
        """Determine if the response should be cached.

        Parameters
        ----------
        request : Request
            The incoming request.
        response : Response
            The response to evaluate.

        Returns
        -------
        bool
            True if the response should be cached, False otherwise.
        """
        # Don't cache if path is excluded
        if request.url.path in self.exclude_paths:
            return False
        
        # Only cache specific HTTP methods
        if request.method not in self.cacheable_methods:
            return False
        
        # Only cache successful responses
        if response.status_code not in self.cacheable_status_codes:
            return False
        
        # Don't override existing Cache-Control headers
        if "Cache-Control" in response.headers:
            return False
        
        return True

        
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        """Process the request and conditionally set the `Cache-Control` header.

        Parameters
        ----------
        request : Request
            The incoming request.
        call_next : RequestResponseEndpoint
            The next middleware or route handler in the processing chain.

        Returns
        -------
        Response
            The response object with the `Cache-Control` header set if applicable.

        Notes
        -----
        This method is automatically called by Starlette for each request-response cycle.
        """
        response: Response = await call_next(request)
        
        if self._should_cache(request, response):
            response.headers["Cache-Control"] = f"public, max-age={self.max_age}"
        
        return response
