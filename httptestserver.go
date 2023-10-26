// # Description
//
// The package provides a HTTP server implementation which can be used to mock HTTP responses and
// spy on incoming requests and outgoing responses. The primary goal of this package is to provide
// an easy way to perform local end-to-end tests of HTTP clients against a local HTTP server.
//
// # Features
//   - Built on top of http and httptest packages.
//   - Easily add predefined HTTP responses.
//   - Responses are served in a FIFO fashion until there is only one left: If only one response is
//     available, it is served indefinitly. The server returns an empty 404 response when no
//     predefined responses are available.
//   - The server records HTTP requests, body and HTTP response in a FIFO fashion. These records can
//     be extracted from the test server to spy on exchanged requests and responses.
//   - In case the server encounter an error while processing the request or serving the predefined
//     response, the server will reply with a 500 response with a text body that is the string
//     representation of the error. The server will also add a record to its queue. The added record
//     will have its ServerError set with an error which wraps the error that has occured.
//   - Helper functions are available to clear responses and records.
//   - Pluggable httptest.Server. The server handler will be overriden by the framework. The
//     underlying httptest.Server is accessible so more experienced users can build more complex
//     test cases (like shutting down client connections, testing with TLS, ...).
package gosette

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

// Data of a predefined server response
type PredefinedServerResponse struct {
	// HTTP status code to return
	Status int
	// Headers to return
	Headers http.Header
	// Body to return
	Body []byte
}

// Data of a server record. The server save in a record each incoming request and the corresponding
// HTTP response. The server also save a copy of the request body if any.
//
// In case the test server failed to process
type ServerRecord struct {
	// The HTTP request received by the test server.
	//
	// The body of the request is closed by the test server. Use the RequestBody in the record to
	// get a copy of the body of the HTTP request.
	Request *http.Request
	// A recorder used to record the HTTP response sent by the test server. Never nil.
	Response *httptest.ResponseRecorder
	// A copy of the request body. Will be empty in case request has no body. Never nil.
	RequestBody *bytes.Buffer
	// This member will be non-nil only in case an error has occured while handling the incoming
	// request. The member will contain an error which wraps the error that has occured.
	ServerError error
}

// HTTP test server used to mock real HTTP servers.
//
// Predefined responses and recorded requests are voluntary left public to
// allow users to navigate and manage their data.
type HTTPTestServer struct {
	// Instance of httptest.Server which mocks a real HTTP server and records exchanged data.
	server *httptest.Server
	// Predefined responses. Responses are provided once in a FIFO fashion. If there is only one
	// response left, this response is served indefinitly. In case no predefined responses are
	// available, an HTTP response with a 404 status code and an empty body will be returned.
	responses []*PredefinedServerResponse
	// Recorded requests and responses. Records are appended to the queue in a FIFO fashion.
	records []*ServerRecord
}

// The test server handler which records incoming requests, request body and outgoing responses.
//
// Predefined responses are served once in a FIFO fashion. When there is only one response left in
// predefined response the queue, this response is served indefinitly. When no responses are
// available, the test server replies with an empty 404 response.
func (srv *HTTPTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Prepare response recorder and server record
	responseRecorder := httptest.NewRecorder()
	serverRecord := &ServerRecord{
		Request:     r,
		Response:    responseRecorder,
		RequestBody: &bytes.Buffer{},
		ServerError: nil,
	}

	// Create a multi target ResponseWriter to write response to both the recorder and the client
	// connection. Put the recorder as first so it will always record the response even in case
	// the server fails to write the response to the client connection.
	mw := newMultiTargetHTTPResponseWriter(responseRecorder, w)

	// Create a TeeReader to spy on body when it will be read.
	r.Body = io.NopCloser(io.TeeReader(r.Body, serverRecord.RequestBody))

	// Copy body if any and if content-type is not application/x-www-form-urlencoded
	if r.Body != nil && r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		// Read body, tee reader will automatically copy data to buffer
		_, err := io.ReadAll(r.Body)
		if err != nil {
			// Create an error which wraps the error that has occured
			werr := fmt.Errorf("test server failed to read the request body: %w", err)
			// Handle the error and return a 500 response
			srv.handleInternalError(mw, serverRecord, werr)
			// Exit
			return
		}
	}

	// Parse request query string and body in case content-type is application/x-www-form-urlencoded
	err := r.ParseForm()
	if err != nil {
		// Create an error which wraps the error that has occured
		werr := fmt.Errorf("test server failed to parse query string and form data: %w", err)
		// Handle the error and return a 500 response
		srv.handleInternalError(mw, serverRecord, werr)
		// Exit
		return
	}

	// Build default response
	response := &PredefinedServerResponse{
		Status: http.StatusNotFound,
	}

	// Get first predefined response in the queue if any
	if len(srv.responses) >= 1 {
		response = srv.responses[0]
	}

	// If there are other predefined responses in the queue, pop the used response
	// Keep otherwise
	if len(srv.responses) > 1 {
		srv.responses = srv.responses[1:]
	}

	// Write response headers
	for header, values := range response.Headers {
		for _, value := range values {
			mw.headersAdd(header, value)
		}
	}

	// Write status code
	mw.WriteHeader(response.Status)

	// Write body if any
	if len(response.Body) > 0 {
		_, err := mw.Write(response.Body)
		if err != nil {
			// Create an error which wraps the error that has occured
			werr := fmt.Errorf("test server failed to write the predefined response: %w", err)
			// Handle the error and return a 500 response
			srv.handleInternalError(mw, serverRecord, werr)
			// Exit
			return
		}
	}

	// Success - Add the server record and exit
	srv.records = append(srv.records, serverRecord)
}

// # Description
//
// Factory to create a new, unstarted HTTPTestServer. The underlying httptest.Server can be
// provided by the user in case specific server cofnigurations (TLS, ...) must be used.
//
// # Inputs
//
//   - server: The underlying httptest.Server to be used by the HTTPTestServer. In case it is nil,
//     a new unstarted httptest.Server with default settings will be created.
func NewHTTPTestServer(server *httptest.Server) *HTTPTestServer {
	// Use a default httptest server if nil is provided
	if server == nil {
		server = httptest.NewUnstartedServer(nil)
	}
	// Create HTTPTestServer to return.
	r := &HTTPTestServer{
		server:    server,
		responses: []*PredefinedServerResponse{},
		records:   []*ServerRecord{},
	}
	// Use the HTTPTestServer
	server.Config.Handler = r
	return r
}

// Start the test server.
func (hts *HTTPTestServer) Start() {
	hts.server.Start()
}

// Start the test server with TLS activated.
func (hts *HTTPTestServer) StartTLS() {
	hts.server.StartTLS()
}

// Close the http test server
func (hts *HTTPTestServer) Close() {
	hts.server.Close()
}

func (hts *HTTPTestServer) Client() *http.Client {
	return hts.server.Client()
}

// Get the underlying httptest.Server used by this HTTPTestServer.
func (hts *HTTPTestServer) GetUnderlyingHTTPTestServer() *httptest.Server {
	return hts.server
}

// Return the test server base URL of form http://ipaddr:port with no trailing slash.
func (hts *HTTPTestServer) GetBaseURL() string {
	return hts.server.URL
}

// Push a predefined response to the server.
func (hts *HTTPTestServer) PushPredefinedServerResponse(resp *PredefinedServerResponse) {
	hts.responses = append(hts.responses, resp)
}

// Pop a server record (received request and response) if any. Server records are recorded and
// provided in a FIFO fashion. The returned record will be nil if no record is available.
func (hts *HTTPTestServer) PopServerRecord() *ServerRecord {
	// Prepare return value
	var record *ServerRecord = nil
	// Pop first record if any
	if len(hts.records) >= 1 {
		record, hts.records = hts.records[0], hts.records[1:]
	}
	// Return server record
	return record
}

// Clear all predefined responses configured for the http test server
func (hts *HTTPTestServer) ClearPredefinedServerResponses() {
	hts.responses = []*PredefinedServerResponse{}
}

// Clear all test server records
func (hts *HTTPTestServer) ClearServerRecords() {
	hts.records = []*ServerRecord{}
}

// Clear all server predefined responses & records
func (hts *HTTPTestServer) Clear() {
	hts.ClearPredefinedServerResponses()
	hts.ClearServerRecords()
}

// Helper method which records an error into the provided serverRecord, add the server record to
// the record queue and writea 500 response with the error as text body by using the provided
// http.ResponseWriter.
func (srv *HTTPTestServer) handleInternalError(w http.ResponseWriter, serverRecord *ServerRecord, err error) {
	// Add the error to the server record
	serverRecord.ServerError = err
	// Add the server record to the queue of records
	srv.records = append(srv.records, serverRecord)
	// Send a 500 response with the wrapped error as text as response body
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}
