// SPDX-License-Identifier: GPL-3.0-or-later
import { createServer } from 'node:http';

import { ROOT_CONTEXT, SpanKind, propagation, trace } from '@opentelemetry/api';
import { logs } from '@opentelemetry/api-logs';
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-http';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { BatchLogRecordProcessor, LoggerProvider } from '@opentelemetry/sdk-logs';
import { BatchSpanProcessor, NodeTracerProvider } from '@opentelemetry/sdk-trace-node';
import * as Sentry from '@sentry/node';

const SERVICE = 'sample-js';
const VERSION = '0.1.0';

const resource = resourceFromAttributes({
  'service.name': SERVICE,
  'deployment.environment': 'development',
  'service.version': VERSION,
});

const tracerProvider = new NodeTracerProvider({
  resource,
  spanProcessors: [new BatchSpanProcessor(new OTLPTraceExporter())],
});
tracerProvider.register();

const loggerProvider = new LoggerProvider({
  resource,
  processors: [new BatchLogRecordProcessor({ exporter: new OTLPLogExporter() })],
});
logs.setGlobalLoggerProvider(loggerProvider);

Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: 'development',
  release: `${SERVICE}@${VERSION}`,
  // Dedupe compares the first exception, the constant cause, so it would drop every later handled event.
  integrations: (defaults) => [...defaults.filter((integration) => integration.name !== 'Dedupe'), Sentry.openTelemetryIntegration()],
});
Sentry.setTag('service', SERVICE);

const tracer = trace.getTracer(SERVICE);
const logger = logs.getLogger(SERVICE);

function message(text, variant) {
  return variant === null ? text : `${text} (variant v${variant})`;
}

function handled(variant) {
  const traceId = trace.getActiveSpan().spanContext().traceId;
  logger.emit({ severityText: 'INFO', body: 'handling a request that fails with a handled error' });
  Sentry.addBreadcrumb({ category: 'app', message: 'about to fail', level: 'info' });
  const cause = new TypeError('order total is not a number');
  const eventId = Sentry.captureException(new Error(message('handled failure', variant), { cause }));
  return { trace_id: traceId, event_id: eventId };
}

function route(url, headers) {
  if (url.pathname === '/healthz') {
    return { status: 'ok' };
  }
  if (url.pathname !== '/handled') {
    return null;
  }
  const parent = propagation.extract(ROOT_CONTEXT, headers);
  return tracer.startActiveSpan('GET /handled', { kind: SpanKind.SERVER }, parent, (span) => {
    try {
      return handled(url.searchParams.get('variant'));
    } finally {
      span.end();
    }
  });
}

const server = createServer((req, res) => {
  const body = route(new URL(req.url, 'http://localhost'), req.headers);
  res.writeHead(body ? 200 : 404, { 'content-type': 'application/json' });
  res.end(JSON.stringify(body ?? { status: 'not found' }));
});
server.listen(8000);

async function shutdown() {
  await Promise.all([tracerProvider.shutdown(), loggerProvider.shutdown(), Sentry.close(2000)]);
  process.exit(0);
}
process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);
