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
//
// modified by Adalberto Sampaio Junior @adalrsjr1

package srv

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/yaml"

	"istio.io/pkg/log"

	"istio.io/tools/isotope/convert/pkg/graph"
	"istio.io/tools/isotope/convert/pkg/graph/size"
	"istio.io/tools/isotope/convert/pkg/graph/svc"
	"istio.io/tools/isotope/convert/pkg/graph/svctype"
)

// HandlerFromServiceGraphYAML makes a handler to emulate the service with name
// serviceName in the service graph represented by the YAML file at path.
func HandlerFromServiceGraphYAML(path string, serviceName string) (Handler, error,
) {
	tracer := otel.GetTracerProvider().Tracer("serviceName")
	ctx, span := tracer.Start(context.Background(), "internal")
	defer span.End()

	serviceGraph, err := func(ctx context.Context) (serviceGraph graph.ServiceGraph, err error) {
		_, span := tracer.Start(ctx, "serviceGraphFromYAMLFile")
		defer span.End()
		return serviceGraphFromYAMLFile(path)
	}(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return Handler{}, err
	}

	service, err := func(ctx context.Context) (
		service svc.Service, err error) {
		_, span := tracer.Start(ctx, "extractService")
		defer span.End()
		return extractService(serviceGraph, serviceName)
	}(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return Handler{}, err
	}
	_ = logService(service)

	serviceTypes := func(ctx context.Context) map[string]svctype.ServiceType {
		_, span := tracer.Start(ctx, "extractServiceTypes")
		defer span.End()
		return extractServiceTypes(serviceGraph)
	}(ctx)

	responsePayload, err := func(ctx context.Context) ([]byte, error) {
		_, span := tracer.Start(ctx, "makeRandomByteArray")
		defer span.End()
		return makeRandomByteArray(service.ResponseSize)
	}(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return Handler{}, err
	}

	return Handler{
		Service:         service,
		ServiceTypes:    serviceTypes,
		responsePayload: responsePayload,
	}, nil
}

func makeRandomByteArray(n size.ByteSize) ([]byte, error) {
	arr := make([]byte, n)
	if _, err := rand.Read(arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func logService(service svc.Service) error {
	if log.InfoEnabled() {
		serviceYAML, err := yaml.Marshal(service)
		if err != nil {
			return err
		}
		log.Infof("acting as service %s:\n%s", service.Name, serviceYAML)
	}
	return nil
}

// serviceGraphFromYAMLFile unmarshals the ServiceGraph from the YAML at path.
func serviceGraphFromYAMLFile(
	path string) (serviceGraph graph.ServiceGraph, err error) {
	graphYAML, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	log.Debugf("unmarshalling\n%s", graphYAML)
	err = yaml.Unmarshal(graphYAML, &serviceGraph)
	if err != nil {
		return
	}
	return
}

// extractService finds the service in serviceGraph with the specified name.
func extractService(
	serviceGraph graph.ServiceGraph, name string) (
	service svc.Service, err error) {
	for _, svc := range serviceGraph.Services {
		if svc.Name == name {
			service = svc
			return
		}
	}
	err = fmt.Errorf(
		"service with name %s does not exist in %v", name, serviceGraph)
	return
}

// extractServiceTypes builds a map from service name to its type
// (i.e. HTTP or gRPC).
func extractServiceTypes(
	serviceGraph graph.ServiceGraph) map[string]svctype.ServiceType {
	types := make(map[string]svctype.ServiceType, len(serviceGraph.Services))
	for _, service := range serviceGraph.Services {
		types[service.Name] = service.Type
	}
	return types
}
