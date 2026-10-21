"""ASGI entrypoint.

Run with ``uvicorn visper_ai.main:app --host 0.0.0.0 --port 8088`` or the
``visper-ai`` console script.
"""

import uvicorn

from .api import router
from .core.config import settings
from .core.logger import setup_logging
from .core.setup import create_application

setup_logging()

app = create_application(router=router, settings=settings)


def main() -> None:
    uvicorn.run(
        "visper_ai.main:app",
        host=settings.APP_HOST,
        port=settings.APP_PORT,
        reload=False,
        # Keep the structlog handlers installed by setup_logging().
        log_config=None,
    )


if __name__ == "__main__":
    main()
