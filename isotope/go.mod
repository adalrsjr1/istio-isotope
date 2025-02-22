module istio.io/tools/isotope

go 1.14

require (
	github.com/docker/go-units v0.4.0
	github.com/google/uuid v1.1.1
	github.com/prometheus/client_golang v1.5.1
	github.com/spf13/cobra v0.0.7
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.35.0
	go.opentelemetry.io/otel v1.10.0
	go.opentelemetry.io/otel/exporters/jaeger v1.10.0
	go.opentelemetry.io/otel/sdk v1.10.0
	go.opentelemetry.io/otel/trace v1.10.0
	istio.io/pkg v0.0.0-20200327214633-ce134a9bd104
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	sigs.k8s.io/yaml v1.2.0
)
