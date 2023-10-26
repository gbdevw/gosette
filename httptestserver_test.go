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
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

/*************************************************************************************************/
/* TEST SUITE SETUP                                                                              */
/*************************************************************************************************/

// Unit test suite for HTTP Test Server
type HTTPTestServerUnitTestSuite struct {
	// Test suite
	suite.Suite
	// HTTPTestServer used by the test suite
	hts *HTTPTestServer
}

// Run the unit test suite.
func TestMockHTTPServerUnitTestSuite(t *testing.T) {
	suite.Run(t, &HTTPTestServerUnitTestSuite{})
}

// Build and start HTTPTestServer when test suite starts
func (suite *HTTPTestServerUnitTestSuite) SetupSuite() {
	suite.hts = NewHTTPTestServer(nil)
	suite.hts.Start()
}

// Clear test server predefined responses and records after each test
func (suite *HTTPTestServerUnitTestSuite) TearDownTest() {
	suite.hts.Clear()
}

// Close HTTPTestServer before finishing tests
func (suite *HTTPTestServerUnitTestSuite) TearDownSuite() {
	suite.hts.Close()
}

/*************************************************************************************************/
/* TESTS                                                                                         */
/*************************************************************************************************/

// Test HTTPTestServer with a predefined JSON response.
//
// This first test shows a simple usage of the test server and how to use recorded request, body
// and response.
func (suite *HTTPTestServerUnitTestSuite) TestWithSingleJsonResponse() {

	// Content of a predefined response & expectations
	expectedContentType := "application/json"
	predefinedJsonResponse := `
	{
		"id": 1,
		"test": "success"
	}`
	expectedStatusCode := http.StatusOK

	// Push predefined response to server
	suite.hts.PushPredefinedServerResponse(&PredefinedServerResponse{
		Status: expectedStatusCode,
		Headers: map[string][]string{
			"Content-Type": []string{expectedContentType},
		},
		Body: []byte(predefinedJsonResponse),
	})

	// Get a http.Client which trusts the server certificate if TLS is enabled
	client := suite.hts.Client()
	require.NotNil(suite.T(), client)

	// Send a HTTP request to server
	// Server is expected to reply with the predefined response
	resp, err := client.Get(suite.hts.server.URL)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	// Check first response status code
	require.Equal(suite.T(), expectedStatusCode, resp.StatusCode)
	// Check response content type
	require.Equal(suite.T(), expectedContentType, resp.Header.Get("Content-Type"))
	// Read response body and compare with expected first response
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), respBody)
	require.Equal(suite.T(), []byte(predefinedJsonResponse), respBody)
	// Pop server record
	record := suite.hts.PopServerRecord()
	require.NoError(suite.T(), record.ServerError)
	require.NotNil(suite.T(), record.Request)
	require.Empty(suite.T(), record.RequestBody) // Request body is empty aas request is a GET
	require.NotNil(suite.T(), record.Response)
	// Extract recorded response body and compare
	recordedRespBody, err := io.ReadAll(record.Response.Result().Body)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), respBody, recordedRespBody)
	// Compare request method & URL
	require.Contains(suite.T(), suite.hts.server.URL, record.Request.Host)
	require.Equal(suite.T(), http.MethodGet, record.Request.Method)
}

// Test HTTPTestServer when form encoded data are provided in the incoming request. Test will
// ensure test server handles and record well the request form data.
func (suite *HTTPTestServerUnitTestSuite) TestWithFormEncodedData() {}

// Test HTTPTestServer when multiple predefined responses are defined. Test will ensure predefined
// responses and records are served in a FIFO fashion. Test will also ensure last predefined
// response is served indefinitly.
func (suite *HTTPTestServerUnitTestSuite) TestWithMultipleResponses() {}
