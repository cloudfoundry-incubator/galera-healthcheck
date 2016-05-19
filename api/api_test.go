package api_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	healthcheckfakes "github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck/fakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client/fakes"
	sequencefakes "github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

const (
	ExpectedSeqno             = "4"
	ArbitratorSeqnoResponse   = "no sequence number - running on arbitrator node"
	ExpectedHealthCheckStatus = "synced"
	ApiUsername               = "fake-username"
	ApiPassword               = "fake-password"
)

var _ = Describe("Sidecar API", func() {
	var (
		monitClient    *fakes.FakeMonitClient
		sequenceNumber *sequencefakes.FakeSequenceNumberChecker
		healthchecker  *healthcheckfakes.FakeHealthChecker
		ts             *httptest.Server
	)

	BeforeEach(func() {
		monitClient = &fakes.FakeMonitClient{}
		sequenceNumber = &sequencefakes.FakeSequenceNumberChecker{}
		sequenceNumber.CheckReturns(ExpectedSeqno, nil)

		healthchecker = &healthcheckfakes.FakeHealthChecker{}
		healthchecker.CheckReturns(ExpectedHealthCheckStatus, nil)

		testLogger := lagertest.NewTestLogger("mysql_cmd")
		monitClient.GetLoggerReturns(testLogger)

		testConfig := &config.Config{
			SidecarEndpoint: config.SidecarEndpointConfig{
				Username: ApiUsername,
				Password: ApiPassword,
			},
			Logger: testLogger,
		}

		monitClient.StopServiceReturns("Successfully sent stop request", nil)
		monitClient.StartServiceBootstrapReturns("Successfully sent bootstrap request", nil)
		monitClient.StartServiceJoinReturns("Successfully sent join request", nil)
		monitClient.GetStatusReturns("running", nil)

		handler, err := api.NewRouter(api.ApiParameters{
			RootConfig:            testConfig,
			MonitClient:           monitClient,
			SequenceNumberChecker: sequenceNumber,
			Healthchecker:         healthchecker,
		})
		Expect(err).ToNot(HaveOccurred())
		ts = httptest.NewServer(handler)
	})

	AfterEach(func() {
		ts.Close()
	})

	Context("when request has basic auth", func() {

		var createReq = func(endpoint string, method string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest(method, url, nil)
			Expect(err).ToNot(HaveOccurred())

			req.SetBasicAuth(ApiUsername, ApiPassword)
			return req
		}

		It("Calls StopService on the monit client when a stop command is sent", func() {
			req := createReq("stop_mysql", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(monitClient.StopServiceCallCount()).To(Equal(1))
		})

		It("Calls StartService(join) on the monit client when a start command is sent in join mode", func() {
			req := createReq("start_mysql_join", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceJoinCallCount()).To(Equal(1))
		})

		It("Calls StartService(bootstrap) on the monit client when a start command is sent in bootstrap mode", func() {
			req := createReq("start_mysql_bootstrap", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceBootstrapCallCount()).To(Equal(1))
		})

		It("Calls StartService(single_node) on the monit client when a start command is sent in single_node mode", func() {
			req := createReq("start_mysql_single_node", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.StartServiceSingleNodeCallCount()).To(Equal(1))
		})

		It("Calls GetStatus on the monit client when a new GetStatusCmd is created", func() {
			req := createReq("mysql_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(monitClient.GetStatusCallCount()).To(Equal(1))
		})

		It("Calls Checker on the SequenceNumberchecker when a new sequence_number is created", func() {
			req := createReq("sequence_number", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(1))
		})

		It("returns 404 when a request is made to an unsupplied endpoint", func() {
			req := createReq("nonexistent_endpoint", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Context("when request does not have basic auth", func() {
		var createReq = func(endpoint string, method string) *http.Request {
			url := fmt.Sprintf("%s/%s", ts.URL, endpoint)
			req, err := http.NewRequest(method, url, nil)
			Expect(err).ToNot(HaveOccurred())
			return req
		}

		It("requires authentication for /stop_mysql", func() {
			req := createReq("stop_mysql", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StopServiceCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_bootstrap", func() {
			req := createReq("start_mysql_bootstrap", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceBootstrapCallCount()).To(Equal(0))
		})

		It("requires authentication for /start_mysql_join", func() {
			req := createReq("start_mysql_join", "POST")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.StartServiceJoinCallCount()).To(Equal(0))
		})

		It("requires authentication for /mysql_status", func() {
			req := createReq("mysql_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(monitClient.GetStatusCallCount()).To(Equal(0))
		})

		It("requires authentication for /sequence_number", func() {
			req := createReq("sequence_number", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).ToNot(ContainSubstring(ExpectedSeqno))
			Expect(sequenceNumber.CheckCallCount()).To(Equal(0))
		})

		It("Calls Check on the Healthchecker at the root endpoint", func() {
			req := createReq("", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})

		It("Calls Check on the Healthchecker at /galera_status", func() {
			req := createReq("galera_status", "GET")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			responseBody, err := ioutil.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseBody).To(ContainSubstring(ExpectedHealthCheckStatus))
			Expect(healthchecker.CheckCallCount()).To(Equal(1))
		})
	})
})