package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/einfachnuralex/rmm/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

var _ = Describe("Client", func() {

	Describe("runExec", func() {
		It("runs a command and returns its output with exit code 0", func() {
			out, code, err := runExec(map[string]string{"command": "echo hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
			Expect(out).To(ContainSubstring("hello"))
		})

		It("returns a non-zero exit code without wrapping it as an error", func() {
			_, code, err := runExec(map[string]string{"command": "exit 1"})
			Expect(err).NotTo(HaveOccurred(), "non-zero exit should not produce an error")
			Expect(code).To(Equal(1))
		})

		It("returns an error when the 'command' key is missing", func() {
			_, _, err := runExec(map[string]string{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the command is an empty string", func() {
			_, _, err := runExec(map[string]string{"command": ""})
			Expect(err).To(HaveOccurred())
		})

		It("captures stderr in the combined output", func() {
			if runtime.GOOS == "windows" {
				Skip("skipped on Windows")
			}
			out, _, _ := runExec(map[string]string{"command": "echo err >&2"})
			Expect(out).To(ContainSubstring("err"))
		})
	})

	Describe("getResource", func() {
		Context("hostname", func() {
			It("returns the machine hostname", func() {
				out, err := getResource(map[string]string{"name": "hostname"})
				Expect(err).NotTo(HaveOccurred())
				expected, _ := os.Hostname()
				Expect(out).To(Equal(expected))
			})
		})

		Context("os", func() {
			It("returns GOOS/GOARCH", func() {
				out, err := getResource(map[string]string{"name": "os"})
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(Equal(runtime.GOOS + "/" + runtime.GOARCH))
			})
		})

		Context("pid", func() {
			It("returns a non-empty process ID", func() {
				out, err := getResource(map[string]string{"name": "pid"})
				Expect(err).NotTo(HaveOccurred())
				Expect(out).NotTo(BeEmpty())
			})
		})

		Context("env", func() {
			BeforeEach(func() {
				os.Setenv("RMM_TEST_VAR", "hello123")
			})
			AfterEach(func() {
				os.Unsetenv("RMM_TEST_VAR")
			})

			It("returns the value of the requested environment variable", func() {
				out, err := getResource(map[string]string{"name": "env", "key": "RMM_TEST_VAR"})
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(Equal("hello123"))
			})

			It("returns an error when the 'key' field is missing", func() {
				_, err := getResource(map[string]string{"name": "env"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("error cases", func() {
			It("returns an error for an unknown resource name", func() {
				_, err := getResource(map[string]string{"name": "does-not-exist"})
				Expect(err).To(HaveOccurred())
			})

			It("returns an error when the 'name' key is missing", func() {
				_, err := getResource(map[string]string{})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("case-insensitivity", func() {
			It("matches resource names case-insensitively", func() {
				_, err := getResource(map[string]string{"name": "OS"})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("poll", func() {
		It("returns tasks from the server", func() {
			task := api.Task{ID: "task-1", Type: "exec"}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(api.HeartbeatResponse{Tasks: []api.Task{task}})
			}))
			defer ts.Close()

			resp, err := poll(&http.Client{}, ts.URL, "key", "pc-01")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Tasks).To(HaveLen(1))
			Expect(resp.Tasks[0].ID).To(Equal("task-1"))
		})

		It("sends the X-API-Key header", func() {
			var receivedKey string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedKey = r.Header.Get("X-API-Key")
				json.NewEncoder(w).Encode(api.HeartbeatResponse{})
			}))
			defer ts.Close()

			poll(&http.Client{}, ts.URL, "my-secret", "pc-01")
			Expect(receivedKey).To(Equal("my-secret"))
		})

		It("returns an error when the server responds with a non-200 status", func() {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}))
			defer ts.Close()

			_, err := poll(&http.Client{}, ts.URL, "key", "pc-01")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("sendResult", func() {
		It("POSTs the task result to the server", func() {
			var received api.TaskResult
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&received)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			sendResult(&http.Client{}, ts.URL, "key", api.TaskResult{
				TaskID:   "task-1",
				ClientID: "pc-01",
				Output:   "hello",
				ExitCode: 0,
			})

			Expect(received.TaskID).To(Equal("task-1"))
			Expect(received.Output).To(Equal("hello"))
		})

		It("sends the X-API-Key header", func() {
			var receivedKey string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedKey = r.Header.Get("X-API-Key")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			sendResult(&http.Client{}, ts.URL, "secret", api.TaskResult{TaskID: "t1"})
			Expect(receivedKey).To(Equal("secret"))
		})

		It("sends the Content-Type header as application/json", func() {
			var receivedContentType string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedContentType = r.Header.Get("Content-Type")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			sendResult(&http.Client{}, ts.URL, "key", api.TaskResult{TaskID: "t1"})
			Expect(receivedContentType).To(ContainSubstring("application/json"))
		})
	})
})
