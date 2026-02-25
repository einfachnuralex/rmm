package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/einfachnuralex/rmm/internal/api"
	"github.com/einfachnuralex/rmm/internal/store"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}

func newTestServer() *server {
	s, err := store.New("")
	Expect(err).NotTo(HaveOccurred())
	return &server{store: s, apiKey: "test-key"}
}

func buildMux(srv *server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /heartbeat", srv.auth(srv.handleHeartbeat))
	mux.HandleFunc("POST /task-result", srv.auth(srv.handleTaskResult))
	mux.HandleFunc("GET /clients", srv.auth(srv.handleListClients))
	mux.HandleFunc("POST /clients/{id}/task", srv.auth(srv.handleEnqueueTask))
	mux.HandleFunc("GET /tasks/{id}/result", srv.auth(srv.handleGetTaskResult))
	mux.HandleFunc("GET /poll", srv.auth(srv.handlePoll))
	return mux
}

func doRequest(mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		buf = bytes.NewBuffer(data)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

var _ = Describe("Server", func() {

	var (
		srv *server
		mux *http.ServeMux
	)

	BeforeEach(func() {
		srv = newTestServer()
		mux = buildMux(srv)
	})

	Describe("Auth middleware", func() {
		It("rejects requests with no API key", func() {
			req := httptest.NewRequest("GET", "/clients", nil)
			rr := httptest.NewRecorder()
			srv.auth(srv.handleListClients)(rr, req)
			Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		})

		It("rejects requests with a wrong API key", func() {
			req := httptest.NewRequest("GET", "/clients", nil)
			req.Header.Set("X-API-Key", "wrong")
			rr := httptest.NewRecorder()
			srv.auth(srv.handleListClients)(rr, req)
			Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("POST /heartbeat", func() {
		It("accepts a valid heartbeat and returns 200", func() {
			rr := doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{
				ClientID: "pc-01", Hostname: "myhost", OS: "linux", Arch: "amd64",
				Uptime: 500, Load5: 0.25,
			})
			Expect(rr.Code).To(Equal(http.StatusOK))

			var resp api.HeartbeatResponse
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
		})

		It("returns 400 when client_id is missing", func() {
			rr := doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{})
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for malformed JSON", func() {
			req := httptest.NewRequest("POST", "/heartbeat", strings.NewReader("not-json"))
			req.Header.Set("X-API-Key", "test-key")
			rr := httptest.NewRecorder()
			srv.handleHeartbeat(rr, req)
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("includes pending tasks in the response", func() {
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "h"})
			srv.store.AddTask("pc-01", api.Task{ID: "task-1", Type: "exec"})

			rr := doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "h"})
			var resp api.HeartbeatResponse
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Tasks).To(HaveLen(1))
			Expect(resp.Tasks[0].ID).To(Equal("task-1"))
		})
	})

	Describe("GET /clients", func() {
		It("returns an empty list when no clients have connected", func() {
			rr := doRequest(mux, "GET", "/clients", nil)
			Expect(rr.Code).To(Equal(http.StatusOK))

			var clients []api.ClientRecord
			Expect(json.NewDecoder(rr.Body).Decode(&clients)).To(Succeed())
			Expect(clients).To(BeEmpty())
		})

		It("returns the client after a heartbeat, marked as online", func() {
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "myhost"})

			rr := doRequest(mux, "GET", "/clients", nil)
			var clients []api.ClientRecord
			Expect(json.NewDecoder(rr.Body).Decode(&clients)).To(Succeed())
			Expect(clients).To(HaveLen(1))
			Expect(clients[0].ClientID).To(Equal("pc-01"))
			Expect(clients[0].Online).To(BeTrue())
		})
	})

	Describe("POST /clients/{id}/task", func() {
		BeforeEach(func() {
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "h"})
		})

		It("enqueues a task and returns 202 with a task_id", func() {
			rr := doRequest(mux, "POST", "/clients/pc-01/task", api.Task{
				Type:    "exec",
				Payload: map[string]string{"command": "hostname"},
			})
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			var resp map[string]string
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp["task_id"]).NotTo(BeEmpty())
		})

		It("auto-generates a task_id when none is provided", func() {
			rr := doRequest(mux, "POST", "/clients/pc-01/task", api.Task{Type: "exec"})
			var resp map[string]string
			json.NewDecoder(rr.Body).Decode(&resp)
			Expect(resp["task_id"]).To(HavePrefix("task-"))
		})

		It("returns 404 for an unknown client", func() {
			rr := doRequest(mux, "POST", "/clients/nobody/task", api.Task{Type: "exec"})
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("POST /task-result", func() {
		It("accepts a task result and returns 204", func() {
			rr := doRequest(mux, "POST", "/task-result", api.TaskResult{
				TaskID: "task-1", ClientID: "pc-01", Output: "hello", ExitCode: 0,
			})
			Expect(rr.Code).To(Equal(http.StatusNoContent))
		})

		It("returns 400 for malformed JSON", func() {
			req := httptest.NewRequest("POST", "/task-result", strings.NewReader("bad"))
			req.Header.Set("X-API-Key", "test-key")
			rr := httptest.NewRecorder()
			srv.handleTaskResult(rr, req)
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("GET /tasks/{id}/result", func() {
		It("returns the result once it has been submitted", func() {
			srv.store.SaveTaskResult(api.TaskResult{
				TaskID: "task-1", ClientID: "pc-01", Output: "hello", ExitCode: 0,
			})

			rr := doRequest(mux, "GET", "/tasks/task-1/result", nil)
			Expect(rr.Code).To(Equal(http.StatusOK))

			var result api.TaskResult
			Expect(json.NewDecoder(rr.Body).Decode(&result)).To(Succeed())
			Expect(result.Output).To(Equal("hello"))
		})

		It("returns 404 when the result is not yet available", func() {
			rr := doRequest(mux, "GET", "/tasks/missing/result", nil)
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("GET /poll", func() {
		It("returns 400 when client_id is missing", func() {
			rr := doRequest(mux, "GET", "/poll", nil)
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns queued tasks immediately without blocking", func() {
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "h"})
			srv.store.AddTask("pc-01", api.Task{ID: "task-1", Type: "exec"})

			req := httptest.NewRequest("GET", "/poll?client_id=pc-01", nil)
			req.Header.Set("X-API-Key", "test-key")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			var resp api.HeartbeatResponse
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Tasks).To(HaveLen(1))
			Expect(resp.Tasks[0].ID).To(Equal("task-1"))
		})

		It("wakes up and returns the task when one is added while polling", func() {
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{ClientID: "pc-01", Hostname: "h"})

			done := make(chan api.HeartbeatResponse, 1)
			go func() {
				req := httptest.NewRequest("GET", "/poll?client_id=pc-01", nil)
				req.Header.Set("X-API-Key", "test-key")
				rr := httptest.NewRecorder()
				mux.ServeHTTP(rr, req)
				var resp api.HeartbeatResponse
				json.NewDecoder(rr.Body).Decode(&resp)
				done <- resp
			}()

			time.Sleep(20 * time.Millisecond)
			srv.store.AddTask("pc-01", api.Task{ID: "task-wake", Type: "exec"})

			Eventually(done, "2s").Should(Receive(Satisfy(func(resp api.HeartbeatResponse) bool {
				return len(resp.Tasks) == 1 && resp.Tasks[0].ID == "task-wake"
			})))
		})
	})

	Describe("Full round-trip", func() {
		It("completes the full lifecycle: heartbeat → enqueue → poll → submit result → retrieve result", func() {
			By("registering the client via heartbeat")
			doRequest(mux, "POST", "/heartbeat", api.HeartbeatRequest{
				ClientID: "pc-01", Hostname: "myhost", OS: "linux", Arch: "amd64",
			})

			By("enqueueing a task")
			rr := doRequest(mux, "POST", "/clients/pc-01/task", api.Task{
				Type:    "exec",
				Payload: map[string]string{"command": "hostname"},
			})
			var enqResp map[string]string
			json.NewDecoder(rr.Body).Decode(&enqResp)
			taskID := enqResp["task_id"]
			Expect(taskID).NotTo(BeEmpty())

			By("verifying the result is not yet available")
			rr = doRequest(mux, "GET", fmt.Sprintf("/tasks/%s/result", taskID), nil)
			Expect(rr.Code).To(Equal(http.StatusNotFound))

			By("submitting the task result from the client")
			doRequest(mux, "POST", "/task-result", api.TaskResult{
				TaskID:   taskID,
				ClientID: "pc-01",
				Output:   "myhost\n",
				ExitCode: 0,
			})

			By("verifying the result is now retrievable")
			rr = doRequest(mux, "GET", fmt.Sprintf("/tasks/%s/result", taskID), nil)
			Expect(rr.Code).To(Equal(http.StatusOK))
			var result api.TaskResult
			Expect(json.NewDecoder(rr.Body).Decode(&result)).To(Succeed())
			Expect(result.Output).To(Equal("myhost\n"))
			Expect(result.ExitCode).To(Equal(0))
		})
	})
})
