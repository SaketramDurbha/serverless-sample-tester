// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"context"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"google.golang.org/api/idtoken"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// test holds the openapi3.Operation and HTTP method associated with a single
type test struct {
	operation  *openapi3.Operation
	httpMethod string
}

// requestTimeout is the time duration (in seconds) for HTTP requests made to Cloud Run services.
const requestTimeout = 10

// ValidateEndpoints tests all paths (represented by openapi3.Paths) with all HTTP methods and given response bodies
// and make sure they respond with the expected status code. Returns a success bool based on whether all the tests
// passed.
func ValidateEndpoints(serviceURL string, paths *openapi3.Paths) (bool, error) {
	ctx := context.Background()
	httpClient, err := idtoken.NewClient(ctx, serviceURL)
	if err != nil {
		return false, fmt.Errorf("[util.ValidateEndpoints] creating an http.Client: %w", err)
	}
	httpClient.Timeout = time.Second * requestTimeout

	success := true
	for endpoint, pathItem := range *paths {
		log.Printf("Testing %s endpoint\n", endpoint)
		tests := []test{
			{pathItem.Connect, http.MethodConnect},
			{pathItem.Delete, http.MethodDelete},
			{pathItem.Get, http.MethodGet},
			{pathItem.Head, http.MethodHead},
			{pathItem.Options, http.MethodOptions},
			{pathItem.Patch, http.MethodPatch},
			{pathItem.Post, http.MethodPost},
			{pathItem.Put, http.MethodPut},
			{pathItem.Trace, http.MethodTrace},
		}

		endpointURL := serviceURL + endpoint
		for _, t := range tests {
			s, err := validateEndpointOperation(httpClient, endpointURL, t.operation, t.httpMethod)
			if err != nil {
				return s, fmt.Errorf("[util.ValidateEndpoints] testing %s requests on %s: %w", t.httpMethod, endpointURL, err)
			}

			success = s && success
		}
	}

	return success, nil
}

// validateEndpointOperation validates a single endpoint and a single HTTP method, and ensures that the request --
// including the provided sample request body -- elicits the expected status code.
func validateEndpointOperation(client *http.Client, endpointURL string, operation *openapi3.Operation, httpMethod string) (bool, error) {
	if operation == nil {
		return true, nil
	}
	log.Printf("Executing %s %s\n", httpMethod, endpointURL)

	if operation.RequestBody == nil {
		log.Println("Sending empty request body")
		reqBodyReader := strings.NewReader("")

		s, err := makeTestRequest(client, endpointURL, httpMethod, "", reqBodyReader, operation)
		if err != nil {
			return s, fmt.Errorf("[util.validateEndpointOperation] testing %s request on %s: %w", httpMethod, endpointURL, err)
		}

		return s, nil
	}

	reqBodies := operation.RequestBody.Value.Content
	allTestsPassed := true
	for mimeType, mediaType := range reqBodies {
		reqBodyStr := mediaType.Example.(string)
		log.Printf("Sending %s: %s", mimeType, reqBodyStr)

		reqBodyReader := strings.NewReader(reqBodyStr)

		s, err := makeTestRequest(client, endpointURL, httpMethod, mimeType, reqBodyReader, operation)
		if err != nil {
			return s, fmt.Errorf("[util.validateEndpointOperation] testing %s %s request on %s: %w", httpMethod, mimeType, endpointURL, err)
		}

		allTestsPassed = allTestsPassed && s
	}

	return allTestsPassed, nil
}

// makeTestRequest returns a success bool based on whether the returned status code  was included in the provided
// openapi3.Operation expected responses.
func makeTestRequest(client *http.Client, endpointURL, httpMethod, mimeType string, reqBodyReader *strings.Reader, operation *openapi3.Operation) (bool, error) {
	req, err := http.NewRequest(httpMethod, endpointURL, reqBodyReader)
	if err != nil {
		return false, fmt.Errorf("[util.makeTestRequest] creating an http.Request: %w", err)
	}

	req.Header.Add("content-type", mimeType)

	resp, err := (*client).Do(req)
	if err != nil {
		return false, fmt.Errorf("[util.makeTestRequest]: creating executing a http.Request: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("[util.makeTestRequest]: reading http.Response: %w", err)
	}

	statusCode := strconv.Itoa(resp.StatusCode)
	log.Printf("Status code: %s\n", statusCode)

	if val, ok := operation.Responses[statusCode]; ok {
		log.Printf("Response description: %s\n", *val.Value.Description)
		return true, nil
	}

	log.Println("Unknown response description: FAIL")
	log.Println("Dumping response body")
	fmt.Println(string(body))

	return false, nil
}
