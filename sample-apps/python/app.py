# SPDX-License-Identifier: GPL-3.0-or-later
import logging
import os
import threading

import sentry_sdk
from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry._logs import set_logger_provider
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.asgi import OpenTelemetryMiddleware
from opentelemetry.instrumentation.logging.handler import LoggingHandler
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from sentry_sdk.integrations.otlp import OTLPIntegration

SERVICE = "sample-python"
VERSION = "0.1.0"

resource = Resource.create(
    {
        "service.name": SERVICE,
        "deployment.environment": "development",
        "service.version": VERSION,
    }
)

tracer_provider = TracerProvider(resource=resource)
tracer_provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
trace.set_tracer_provider(tracer_provider)

logger_provider = LoggerProvider(resource=resource)
logger_provider.add_log_record_processor(BatchLogRecordProcessor(OTLPLogExporter()))
set_logger_provider(logger_provider)

logging.basicConfig(level=logging.INFO)
logging.getLogger().addHandler(LoggingHandler(logger_provider=logger_provider))
logger = logging.getLogger(SERVICE)

sentry_sdk.init(
    dsn=os.environ["SENTRY_DSN"],
    environment="development",
    release=f"{SERVICE}@{VERSION}",
    integrations=[OTLPIntegration(setup_otlp_traces_exporter=False)],
)
sentry_sdk.set_tag("service", SERVICE)

api = FastAPI()


def _message(text: str, variant: str | None) -> str:
    return text if variant is None else f"{text} (variant v{variant})"


def _fail(variant: str | None) -> None:
    raise RuntimeError(_message("unhandled failure", variant))


def _prepare(variant: str | None) -> None:
    _fail(variant)


@api.get("/healthz")
def healthz():
    return {"status": "ok"}


@api.get("/handled")
def handled(variant: str | None = None):
    trace_id = format(trace.get_current_span().get_span_context().trace_id, "032x")
    logger.info("handling a request that fails with a handled error")
    sentry_sdk.add_breadcrumb(category="app", message="about to fail", level="info")
    try:
        raise ValueError(_message("handled failure", variant))
    except ValueError as exc:
        event_id = sentry_sdk.capture_exception(exc)
    return {"trace_id": trace_id, "event_id": event_id}


@api.get("/error")
def error(variant: str | None = None):
    logger.info("handling a request that fails with an unhandled error")
    _prepare(variant)


@api.post("/no-trace")
def no_trace():
    result: dict[str, str | None] = {}

    def capture() -> None:
        try:
            raise LookupError("failure outside any span")
        except LookupError as exc:
            result["event_id"] = sentry_sdk.capture_exception(exc)

    worker = threading.Thread(target=capture)
    worker.start()
    worker.join()
    return {"event_id": result["event_id"]}


# Outermost, so the request span is still active when Sentry's ASGI middleware captures an unhandled error
app = OpenTelemetryMiddleware(api)
