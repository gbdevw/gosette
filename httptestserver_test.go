package gosette

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
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

	// Content of request
	payload := `{
		"hello": "world"
	}`

	// Push a predefined response to test server
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
	resp, err := client.Post(suite.hts.server.URL, "application/json", strings.NewReader(payload))
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
	require.NotNil(suite.T(), record.Response)
	// Check recorded request body against original payload
	recReqBody, err := io.ReadAll(record.RequestBody)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), payload, string(recReqBody))
	// Extract recorded response body and compare
	recordedRespBody, err := io.ReadAll(record.Response.Result().Body)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), respBody, recordedRespBody)
	// Compare request method & URL
	require.Contains(suite.T(), suite.hts.server.URL, record.Request.Host)
	require.Equal(suite.T(), http.MethodPost, record.Request.Method)
}

// Test HTTPTestServer when form encoded data are provided in the incoming request. Test will
// ensure test server handles and record well the request form data.
func (suite *HTTPTestServerUnitTestSuite) TestWithFormEncodedData() {

	// Predefined form data & expectations
	expectedStatusCode := http.StatusCreated
	expectedUrlPath := "/form"
	predefinedFormData := url.Values{
		"id":       []string{"1"},
		"messages": []string{"hello", "world"},
	}

	// Push a predefined response to test server
	suite.hts.PushPredefinedServerResponse(&PredefinedServerResponse{
		Status:  expectedStatusCode,
		Headers: map[string][]string{},
		Body:    nil,
	})

	// Get a http.Client which trusts the server certificate if TLS is enabled
	client := suite.hts.Client()
	require.NotNil(suite.T(), client)

	// Send a POST request wiith form data
	resp, err := client.PostForm(suite.hts.GetBaseURL()+expectedUrlPath, predefinedFormData)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)

	// Check response
	require.Equal(suite.T(), expectedStatusCode, resp.StatusCode)
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)
	require.Empty(suite.T(), respBody)

	// Extract recorded request and response
	srvrec := suite.hts.PopServerRecord()
	require.NotNil(suite.T(), srvrec)

	// Check request
	require.Equal(suite.T(), http.MethodPost, srvrec.Request.Method)
	require.Equal(suite.T(), expectedUrlPath, srvrec.Request.URL.String())
	require.Equal(suite.T(), predefinedFormData, srvrec.Request.Form)

	// Check recorded request body
	recReqBody, err := io.ReadAll(srvrec.RequestBody)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []byte(predefinedFormData.Encode()), recReqBody)

	// Check recorded response
	require.Equal(suite.T(), resp.StatusCode, srvrec.Response.Result().StatusCode)
	recRespBody, err := io.ReadAll(srvrec.Response.Result().Body)
	require.NoError(suite.T(), err)
	require.Empty(suite.T(), recRespBody)
}

// Test HTTPTestServer when multiple predefined responses are defined. Test will ensure:
//   - An empty 404 response is served when no predefined responses are available
//   - PopServerRecord pops records and returns nil when no records are available
//   - Server records and serves well multiple predefined repsonses in FIFO order.
//   - When only one response is left, it is served indefinitly
//   - ClearServerRecords clears the record queue
//   - ClearPredefinedResponse clears the response queue
func (suite *HTTPTestServerUnitTestSuite) TestWithMultipleResponses() {
	// Get a HTTP client
	client := suite.hts.Client()

	// First, test a request when no predefined repsonse are set
	resp, err := client.Get(suite.hts.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)
	require.Empty(suite.T(), respBody)

	// Pop server record twice. First record must contain the exchanged request-response
	// and the second call must return a nil value
	srvrec := suite.hts.PopServerRecord()
	require.NotNil(suite.T(), srvrec)
	require.Equal(suite.T(), http.StatusNotFound, srvrec.Response.Result().StatusCode)
	srvrec = suite.hts.PopServerRecord()
	require.Nil(suite.T(), srvrec)

	// Push multiple predefined response
	expectedStatusCode1 := http.StatusOK
	expectedStatusCode2 := http.StatusNoContent
	suite.hts.PushPredefinedServerResponse(&PredefinedServerResponse{
		Status:  expectedStatusCode1,
		Headers: map[string][]string{},
		Body:    nil,
	})
	suite.hts.PushPredefinedServerResponse(&PredefinedServerResponse{
		Status:  expectedStatusCode2,
		Headers: map[string][]string{},
		Body:    nil,
	})

	// Send a first request and ensure first response is served
	resp, err = client.Get(suite.hts.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), expectedStatusCode1, resp.StatusCode)

	// Send a second request and ensure second response is served
	resp, err = client.Get(suite.hts.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), expectedStatusCode2, resp.StatusCode)

	// Send a third request and ensure second served is still served
	resp, err = client.Get(suite.hts.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), expectedStatusCode2, resp.StatusCode)

	// Ensure 3 server records are available then clear them and check
	require.NotEmpty(suite.T(), suite.hts.records)
	require.Len(suite.T(), suite.hts.records, 3)
	suite.hts.ClearServerRecords()
	require.Empty(suite.T(), suite.hts.records)

	// Clear server responses and ensure an empty response is now served
	require.NotEmpty(suite.T(), suite.hts.responses)
	suite.hts.ClearPredefinedServerResponses()
	require.Empty(suite.T(), suite.hts.responses)
	resp, err = client.Get(suite.hts.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)
}

// Test HTTPServer with TLS enabled
func (suite *HTTPTestServerUnitTestSuite) TestWithTLSEnabled() {
	// Create a base httptest server
	basetlssrv := httptest.NewUnstartedServer(nil)
	require.NotNil(suite.T(), basetlssrv)
	// Create a separate HTTPTestServer and plug the httptest.Server
	srv := NewHTTPTestServer(basetlssrv)
	require.NotNil(suite.T(), srv)
	// Start the HTTPTestServer with TLS
	srv.StartTLS()
	defer srv.Close()
	// Check BaseURL has https as scheme
	require.Contains(suite.T(), srv.GetBaseURL(), "https")
	// Test GetUnderlyingHTTPTestServer
	tmp := srv.GetUnderlyingHTTPTestServer()
	require.NotNil(suite.T(), tmp)
	require.Equal(suite.T(), basetlssrv.Config.Addr, tmp.Config.Addr)
	// Get a HTTP client with TLS settings
	tlsclient := srv.Client()
	// Send a request with TLS client and expect an empty 404 response from test server
	resp, err := tlsclient.Get(srv.GetBaseURL())
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), resp)
	require.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)
	// Create a default HTTP client
	notlsclient := http.DefaultClient
	// Send a request to base url and expect it to fail (remote errorr: tls: bad certificate)
	resp, err = notlsclient.Get(srv.GetBaseURL())
	require.Error(suite.T(), err)
	require.Nil(suite.T(), resp)
}

// Test handleInternalError
func (suite *HTTPTestServerUnitTestSuite) TestHandleInternalError() {
	// Create a recorder to record response written by handler
	rec := httptest.NewRecorder()
	// Create a server record which will receive the provided error
	srvrec := &ServerRecord{
		Request:     nil,
		Response:    nil,
		RequestBody: &bytes.Buffer{},
		ServerError: nil,
	}
	// Expected error
	eerr := fmt.Errorf("PWNED")
	// Use the error handler
	suite.hts.handleInternalError(rec, srvrec, eerr)
	// Check the server record and compare errors
	require.Equal(suite.T(), eerr, srvrec.ServerError)
	// Check response and expect 500 as status code + error string as text body
	require.Equal(suite.T(), http.StatusInternalServerError, rec.Result().StatusCode)
	require.Equal(suite.T(), "text/plain", rec.Result().Header.Get("Content-Type"))
	recRespBody, err := io.ReadAll(rec.Result().Body)
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), recRespBody)
	require.Equal(suite.T(), eerr.Error(), string(recRespBody))
}

// Test test server handler error paths
func (suite *HTTPTestServerUnitTestSuite) TestServeHTTPErrPaths() {
	// Create a mockReadCloser which fails when body is read
	expectedErr := fmt.Errorf("PWNED")
	mockedReadCloser := mockReadCloser{mock.Mock{}}
	mockedReadCloser.
		On("Read", mock.Anything).Return(0, expectedErr).
		On("Close").Return(nil)
	// Create a mockResponseWriter which fails when response is written
	mockedResponseWriter := mockResponseWriter{mock.Mock{}}
	mockedResponseWriter.
		On("Header").Return(http.Header{}).
		On("WriteHeader", mock.Anything).Return().
		On("Write", mock.Anything).Return(0, expectedErr)
	// Test server handler - fails when reading body
	respRec := httptest.NewRecorder()
	require.NotNil(suite.T(), respRec)
	req := httptest.NewRequest(http.MethodPost, "/", &mockedReadCloser)
	require.NotNil(suite.T(), req)
	suite.hts.ServeHTTP(respRec, req)
	require.Equal(suite.T(), http.StatusInternalServerError, respRec.Result().StatusCode)
	// Test server handler - fails when parsing form data
	respRec = httptest.NewRecorder()
	require.NotNil(suite.T(), respRec)
	req = httptest.NewRequest(http.MethodPost, "/", &mockedReadCloser)
	// Set content type to form encoded so body is read by parseForm
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NotNil(suite.T(), req)
	suite.hts.ServeHTTP(respRec, req)
	require.Equal(suite.T(), http.StatusInternalServerError, respRec.Result().StatusCode)
	// Create a multi target writer with a recorder and the mock which will fail write
	respRec = httptest.NewRecorder()
	require.NotNil(suite.T(), respRec)
	mw := newMultiTargetHTTPResponseWriter(respRec, &mockedResponseWriter)
	require.NotNil(suite.T(), mw)
	// Create a GET request with no body to read
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	require.NotNil(suite.T(), req)
	// Set a predefined response with a body to write
	suite.hts.PushPredefinedServerResponse(&PredefinedServerResponse{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type": []string{"text/plain"},
		},
		Body: []byte("hello world!"),
	})
	// Clear server records
	suite.hts.ClearServerRecords()
	// Call ServeHTTP with the multi target writer
	suite.hts.ServeHTTP(mw, req)
	// Pop record and check error is set and has wrapped the expected error
	record := suite.hts.PopServerRecord()
	require.ErrorAs(suite.T(), record.ServerError, &expectedErr)
}

// Test MultiTargetResponseWriter Header method when no targets are set.
//
// This is to complete test coverage.
func (suite *HTTPTestServerUnitTestSuite) TestMultiTargetResponseWriterHeaderWhenNoTargets() {
	mw := newMultiTargetHTTPResponseWriter()
	require.NotNil(suite.T(), mw)
	headers := mw.Header()
	require.NotNil(suite.T(), headers)
	require.Equal(suite.T(), http.Header{}, headers)
}

/*************************************************************************************************/
/* READCLOSER MOCK                                                                               */
/*************************************************************************************************/

// Mock for ReadCloser interface
type mockReadCloser struct {
	mock.Mock
}

// Mocked Read method
func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

// Mocked Close method
func (m *mockReadCloser) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Test mockReadCloser complies to io.ReadCloser interface
func TestMockReadCloserInterfaceCompliance(t *testing.T) {
	var instance interface{} = &mockReadCloser{}
	_, ok := instance.(io.ReadCloser)
	require.True(t, ok)
}

/*************************************************************************************************/
/* RESPONSE WRITER MOCK                                                                          */
/*************************************************************************************************/

// Mock for http.ResponseWriter interface
type mockResponseWriter struct {
	mock.Mock
}

// Mocked Header method
func (m *mockResponseWriter) Header() http.Header {
	args := m.Called()
	return args.Get(0).(http.Header)
}

// Mocked Write method
func (m *mockResponseWriter) Write(data []byte) (int, error) {
	args := m.Called(data)
	return args.Int(0), args.Error(1)
}

// Mocked WriteHeader method
func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.Called(statusCode)
}

// Test mockResponseWriter complies to http.ResponseWriter interface
func TestMockResponseWriterInterfaceCompliance(t *testing.T) {
	var instance interface{} = &mockResponseWriter{}
	_, ok := instance.(http.ResponseWriter)
	require.True(t, ok)
}
