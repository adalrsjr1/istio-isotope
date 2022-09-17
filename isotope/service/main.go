// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this currentFile except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path"
	"runtime"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"istio.io/pkg/log"

	"istio.io/tools/isotope/convert/pkg/consts"
	"istio.io/tools/isotope/service/pkg/srv"
	"istio.io/tools/isotope/service/pkg/srv/prometheus"
)

const (
	promEndpoint    = "/metrics"
	defaultEndpoint = "/"
)

var (
	serviceGraphYAMLFilePath = path.Join(
		consts.ConfigPath, consts.ServiceGraphYAMLFileName)

	// Set by the Python script when running this through Docker.
	maxIdleConnectionsPerHostFlag = flag.Int(
		"max-idle-connections-per-host", 0,
		"maximum number of TCP connections to keep open per host")

	// Set log levels
	logLevel = flag.String(
		"log-level", "info",
		"log level")
)

var stringToLevel = map[string]log.Level{
	"debug": log.DebugLevel,
	"info":  log.InfoLevel,
	"warn":  log.WarnLevel,
	"error": log.ErrorLevel,
	"fatal": log.FatalLevel,
	"none":  log.NoneLevel,
}

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func tracerProvider(url, service, environment, podname, nodename string, id int64) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
			attribute.String("environment", environment),
			attribute.Int64("ID", id),
			attribute.String("pod", podname),
			attribute.String("node", nodename),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

func help() {
	fmt.Println(`List of Env Vars required and default values`)
	fmt.Println(`JAEGERADDR: jaeger-collector.kube-monitoring`)
	fmt.Println(`JAEGERPORT: 14268`)
	fmt.Println(`PODNAME: auto-inject using K8S downward API`)
	fmt.Println(`NODENAME: auto-inject using K8S downward API`)
	fmt.Println(`NOTRACE: if this EnvVar exists, even if it is empty, otel tracing will be disable`)
}

func main() {
	help()
	serviceName, ok := os.LookupEnv(consts.ServiceNameEnvKey)
	if !ok {
		log.Fatalf(`env var "%s" is not set`, consts.ServiceNameEnvKey)
	}

	environment := consts.ServiceGraphNamespace
	id := rand.Int63()

	jaegerAddr, ok := os.LookupEnv("JAEGERADDR")
	if !ok {
		jaegerAddr = "jaeger-collector.kube-monitoring"
		// log.Fatalf(`env var "%s" is not set`, consts.ServiceNameEnvKey)
	}

	jaegerPort, ok := os.LookupEnv("JAEGERPORT")
	if !ok {
		jaegerPort = "14268"
		// log.Fatalf(`env var "%s" is not set`, consts.ServiceNameEnvKey)
	}

	podname, ok := os.LookupEnv("PODNAME")
	if !ok {
		log.Fatalf(`env var PODNAME is not set`)
	}

	nodename, ok := os.LookupEnv("NODENAME")
	if !ok {
		log.Fatalf(`env var NODENAME is not set`)
	}

	_, notracing := os.LookupEnv("NOTRACING")

	tp, err := tracerProvider(fmt.Sprintf("http://%s:%s/api/traces", jaegerAddr, jaegerPort), serviceName, environment, podname, nodename, id)

	if err != nil {
		log.Fatalf("%s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cleanly shutdown and flush telemetry when the application exits.
	shutdowntp := func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatalf("%s", err)
		}
	}
	if notracing {
		shutdowntp(ctx)
	}
	defer shutdowntp(ctx)

	flag.Parse()
	for _, s := range log.Scopes() {
		s.SetOutputLevel(stringToLevel[*logLevel])
	}

	setMaxProcs()
	setMaxIdleConnectionsPerHost(*maxIdleConnectionsPerHostFlag)
	defaultHandler, err := srv.HandlerFromServiceGraphYAML(
		serviceGraphYAMLFilePath, serviceName)
	if err != nil {
		log.Fatalf("%s", err)
	}

	_, span := tp.Tracer("service").Start(context.Background(), "main")
	defer span.End()

	tracingHandler := otelhttp.NewHandler(defaultHandler, "defaultHandler", otelhttp.WithTracerProvider(tp))
	err = serveWithPrometheus(tracingHandler)
	if err != nil {
		log.Fatalf("%s", err)
	}
}

func serveWithPrometheus(defaultHandler http.Handler) error {
	log.Infof(`exposing Prometheus endpoint "%s"`, promEndpoint)
	http.Handle(promEndpoint, prometheus.Handler())

	log.Infof(`exposing default endpoint "%s"`, defaultEndpoint)
	http.Handle(defaultEndpoint, defaultHandler)
	addr := fmt.Sprintf(":%d", consts.ServicePort)
	log.Infof("listening on port %v\n", consts.ServicePort)
	if err := http.ListenAndServe(addr, nil); err != nil {
		return err
	}
	return nil
}

func setMaxProcs() {
	numCPU := runtime.NumCPU()
	maxProcs := runtime.GOMAXPROCS(0)
	if maxProcs < numCPU {
		log.Infof("setting GOMAXPROCS to %v (previously %v)", numCPU, maxProcs)
		runtime.GOMAXPROCS(numCPU)
	}
}

func setMaxIdleConnectionsPerHost(n int) {
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = n
}
