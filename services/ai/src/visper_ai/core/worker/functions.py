import asyncio
import logging
from typing import Any

import structlog

logger = logging.getLogger(__name__)


# -------- background tasks --------
async def sample_background_task(ctx: dict[str, Any], name: str) -> str:
    await asyncio.sleep(5)
    return f"Task {name} is complete!"


# -------- base functions --------
async def startup(ctx: dict[str, Any]) -> None:
    logger.info("Worker Started")


async def shutdown(ctx: dict[str, Any]) -> None:
    logger.info("Worker end")


async def on_job_start(ctx: dict[str, Any]) -> None:
    structlog.contextvars.bind_contextvars(job_id=ctx["job_id"])
    logger.info("Job Started")


async def on_job_end(ctx: dict[str, Any]) -> None:
    logger.info("Job Completed")
    structlog.contextvars.clear_contextvars()
