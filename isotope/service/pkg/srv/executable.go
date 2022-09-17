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

package srv

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"istio.io/pkg/log"
	"istio.io/tools/isotope/convert/pkg/graph/script"
	"istio.io/tools/isotope/convert/pkg/graph/svctype"
	"istio.io/tools/isotope/service/pkg/srv/prometheus"
)

var tracer = otel.Tracer("service")

func init() {
	rand.Seed(time.Now().UnixNano())
}

func execute(
	ctx context.Context,
	step interface{},
	forwardableHeader http.Header,
	serviceTypes map[string]svctype.ServiceType) error {

	ctx, span := tracer.Start(ctx, "execute", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	switch cmd := step.(type) {
	case script.SleepCommand:
		executeSleepCommand(ctx, cmd)
	case script.RequestCommand:
		if err := executeRequestCommand(
			ctx, cmd, forwardableHeader, serviceTypes); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	case script.ConcurrentCommand:
		if err := executeConcurrentCommand(
			ctx, cmd, forwardableHeader, serviceTypes); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	default:
		err := fmt.Errorf("unknown command type in script: %T", cmd)
		log.Fatalf(err.Error())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return nil
}

func executeSleepCommand(ctx context.Context, cmd script.SleepCommand) {
	_, span := tracer.Start(ctx, "execute-sleep-command", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	time.Sleep(time.Duration(cmd))
}

func shouldSkipRequest(cmd script.RequestCommand) bool {
	// Probability not set, always send a request
	if cmd.Probability == 0 {
		return false
	}
	return rand.Intn(100) < (100 - cmd.Probability)
}

// Execute sends an HTTP request to another service. Assumes DNS is available
// which maps exe.ServiceName to the relevant URL to reach the service.
func executeRequestCommand(
	ctx context.Context,
	cmd script.RequestCommand,
	forwardableHeader http.Header,
	serviceTypes map[string]svctype.ServiceType,
) error {
	ctx, span := tracer.Start(ctx, "execute-sleep-command", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	if shouldSkipRequest(cmd) {
		span.AddEvent("skip-request")
		return nil
	}

	destName := cmd.ServiceName
	_, ok := serviceTypes[destName]
	if !ok {
		err := fmt.Errorf("service %s does not exist", destName)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	response, err := sendRequest(ctx, destName, cmd.Size, forwardableHeader)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Necessary for reusing HTTP/1.x "keep-alive" TCP connections.
	// https://golang.org/pkg/net/http/#Response
	defer func() {
		// Drain and close the body to let the Transport reuse the connection
		io.Copy(ioutil.Discard, response.Body)
		response.Body.Close()
		prometheus.RecordRequestSent(destName, uint64(cmd.Size))
	}()

	log.Debugf("%s responded with %s", destName, response.Status)
	if response.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			body = []byte(fmt.Sprintf("failed to read body: %v", err))
		}
		err = fmt.Errorf(
			"service %s responded with %s (body: %v)", destName, response.Status, string(body))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return nil
}

// executeConcurrentCommand calls each command in exe.Commands asynchronously
// and waits for each to complete.
func executeConcurrentCommand(
	ctx context.Context,
	cmd script.ConcurrentCommand,
	forwardableHeader http.Header,
	serviceTypes map[string]svctype.ServiceType) error {

	ctx, span := tracer.Start(ctx, "execute-concurrent-command", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	numSubCmds := len(cmd)
	wg := sync.WaitGroup{}
	wg.Add(numSubCmds)
	var errs []string
	for _, subCmd := range cmd {
		go func(step interface{}) {
			defer wg.Done()

			err := execute(ctx, step, forwardableHeader, serviceTypes)
			if err != nil {
				errs = append(errs, err.Error())
			}
		}(subCmd)
	}
	wg.Wait()
	if len(errs) == 0 {
		return nil
	}
	err := fmt.Errorf("%d errors occurred: %v", len(errs), strings.Join(errs, ", "))
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return err
}
