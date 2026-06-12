package common

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func InjectTraceContext(ctx context.Context, headers map[string]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))
}

func ExtractTraceContext(ctx context.Context, headers map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))
}

func StartSpanFromNATSMsg(ctx context.Context, operationName string, headers map[string]string) (context.Context, trace.Span) {
	ctx = ExtractTraceContext(ctx, headers)
	return Tracer("nats").Start(ctx, operationName)
}
