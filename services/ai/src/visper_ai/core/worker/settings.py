"""ARQ worker settings.

Run with ``arq visper_ai.core.worker.settings.WorkerSettings`` or the
``visper-ai-worker`` console script.
"""

import asyncio
import sys
from typing import cast

from arq.connections import RedisSettings
from arq.typing import WorkerSettingsType
from arq.worker import check_health, run_worker

from ..config import settings
from ..logger import setup_logging
from .functions import on_job_end, on_job_start, sample_background_task, shutdown, startup

setup_logging()


class WorkerSettings:
    functions = [sample_background_task]
    # The password was missing, so the worker could not authenticate against a
    # password-protected Redis (as in docker-compose) while the API could.
    redis_settings = RedisSettings(
        host=settings.REDIS_QUEUE_HOST,
        port=settings.REDIS_QUEUE_PORT,
        password=settings.REDIS_QUEUE_PASSWORD,
    )
    on_startup = startup
    on_shutdown = shutdown
    on_job_start = on_job_start
    on_job_end = on_job_end
    # arq installs SIGINT/SIGTERM handlers for graceful shutdown (and skips them on
    # Windows itself); disabling them made `docker stop` kill jobs mid-flight.
    handle_signals = True


def start_arq_service(check: bool = False, burst: bool | None = None, watch: str | None = None) -> None:
    worker_settings_ = cast("WorkerSettingsType", WorkerSettings)

    if check:
        sys.exit(check_health(worker_settings_))
    elif watch:
        from arq.cli import watch_reload  # requires the optional `watchfiles` package

        asyncio.run(watch_reload(watch, worker_settings_))
    else:
        kwargs = {} if burst is None else {"burst": burst}
        run_worker(worker_settings_, **kwargs)


if __name__ == "__main__":
    # python -m visper_ai.core.worker.settings
    start_arq_service()
