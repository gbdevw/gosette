[![Go Reference](https://pkg.go.dev/badge/github.com/gbdevw/gosette.svg)](https://pkg.go.dev/github.com/gbdevw/gosette)
![Coverage](https://img.shields.io/badge/Coverage-100.0%25-brightgreen)
# Gosette

A HTTP server implementation which can be used to mock HTTP responses and spy on incoming HTTP requests and outgoing HTTP responses. The primary goal of this package is to provide an easy way to perform local end-to-end tests of HTTP clients against a local HTTP server.

## Features

- Built on top of http and httptest packages.
- Easily add predefined HTTP responses.
- Responses are served in a FIFO fashion until there is only one left: If only one response is available, it is served indefinitly. The server returns an empty 404 response when no predefined responses are available.
- The server records HTTP requests, body and HTTP response in a FIFO fashion. These records can be extracted from the test server to spy on exchanged requests and responses.
- In case the server encounter an error while processing the request or serving the predefined response, the server will reply with a 500 response with a text body that is the string representation of the error. The server will also add a record to its queue. The added record will have its ServerError set with an error which wraps the error that has occured.
- Helper functions are available to clear responses and records.
- Pluggable httptest.Server. The server handler will be overriden by the framework. The underlying httptest.Server is accessible so more experienced users can build more complex test cases (like shutting down client connections, testing with TLS, ...).

## Basic usage

```go
// Build HTTP Server with default options and start it
testsrv := NewHTTPTestServer(nil)
testsrv.Start()

// Configure a predefined response that will be served indefinitly
testsrv.PushPredefinedServerResponse(&PredefinedServerResponse{
	Status: http.StatusOK,
	Headers: map[string][]string{
		"Content-Type": []string{"application/json"},
	},
	Body: []byte(`{"test": "success"}`),
})

// Get a http.Client which trusts the server certificate if TLS is enabled
client := testsrv.Client()

// Send a request to the http test server
resp, err := client.Get(testsrv.GetBaseURL())
...

// Pop recorded request, request body and recorded response
record := testsrv.PopServerRecord()

// Inspect recorded http.Request
host := record.Request.Host
method := record.Request.Method
path := record.Request.URL.String()
...

// Inspect recorded request body
recReqBody, err := io.ReadAll(record.RequestBody)
...

// Inspect recorded http.Response
recResp := record.Response.Result()
...
```

## Advanced usage and integration with ther testing framework

More advanced usage of the gosette.HTTPTestServer can be seen in the [test file](httptestserver_test.go). Tests also show how gosette.HTTPTestServer can be used in combination with the basic Golang testing utilities as well as the [stretchr/testify](https://github.com/stretchr/testify) testing framework.