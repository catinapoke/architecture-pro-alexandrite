package main

import (
	"context"
	"io"
	"log"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	tracer := Init(context.Background(), "service-a")

	app := fiber.New()

	app.Get("/", func(c fiber.Ctx) error {
		ctx := c.Context()

		propagators := propagation.TraceContext{}
		ctx = propagators.Extract(ctx, newFasthttpHeaderCarrier(&c.Request().Header))

		ctx, span := tracer.Start(ctx, "test-a")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", c.OriginalURL()),
			attribute.String("http.status_code", "200"),
		)

		req, err := http.NewRequest("GET", "http://service-b:8080/", nil)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			return c.SendStatus(http.StatusInternalServerError)
		}

		propagators.Inject(ctx, propagation.HeaderCarrier(req.Header))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			return c.SendStatus(http.StatusInternalServerError)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			return c.SendStatus(http.StatusInternalServerError)
		}

		return c.SendString("Got: " + string(body))
	})

	log.Fatal(app.Listen(":8080"))

}

func newFasthttpHeaderCarrier(headers *fasthttp.RequestHeader) propagation.HeaderCarrier {
	carrier := propagation.HeaderCarrier{}
	for key, value := range headers.All() {
		http.Header(carrier).Add(string(key), string(value))
	}

	return carrier
}
