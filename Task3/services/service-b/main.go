package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/propagation"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

const jaegerEndpoint = "simplest-collector:4317"

// Init returns an instance of Jaeger Tracer.
func Init(ctx context.Context, service string) trace.Tracer {
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(jaegerEndpoint),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		log.Fatal("creating OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(service)),
	)

	return tp.Tracer(service)
}

func newResource(service string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(service),
		semconv.ServiceVersion("0.0.1"),
	)
}

func main() {
	tracer := Init(context.Background(), "service-b")

	app := fiber.New()

	app.Get("/", func(c fiber.Ctx) error {
		ctx := c.Context()

		propagators := propagation.TraceContext{}
		ctx = propagators.Extract(ctx, newFasthttpHeaderCarrier(&c.Request().Header))

		_, span := tracer.Start(ctx, "test-b")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", c.OriginalURL()),
			attribute.String("http.status_code", "200"),
		)

		return c.SendString("Hello, World!")
	})

	log.Fatal(app.Listen(":8080"))

}

func newFasthttpHeaderCarrier(headers *fasthttp.RequestHeader) propagation.HeaderCarrier {
	carrier := propagation.HeaderCarrier{}
	for key, value := range headers.All() {
		fmt.Println("key: ", string(key), " value: ", string(value))
		http.Header(carrier).Add(string(key), string(value))
	}

	return carrier
}
