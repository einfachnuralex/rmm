package store_test

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/einfachnuralex/rmm/internal/api"
	"github.com/einfachnuralex/rmm/internal/store"
)

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Suite")
}

func newStore() *store.Store {
	s, err := store.New("")
	Expect(err).NotTo(HaveOccurred())
	return s
}

func heartbeat(clientID string) api.HeartbeatRequest {
	return api.HeartbeatRequest{
		ClientID: clientID,
		Hostname: clientID + "-host",
		OS:       "linux",
		Arch:     "amd64",
		Uptime:   100,
		Load5:    0.5,
	}
}

var _ = Describe("Store", func() {

	Describe("Upsert and Get", func() {
		It("creates a new client on first Upsert", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))

			r, ok := s.Get("pc-01")
			Expect(ok).To(BeTrue())
			Expect(r.Hostname).To(Equal("pc-01-host"))
			Expect(r.Load5).To(Equal(0.5))
		})

		It("updates an existing client on subsequent Upsert", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))

			updated := heartbeat("pc-01")
			updated.Hostname = "renamed-host"
			updated.Load5 = 1.23
			s.Upsert(updated)

			r, _ := s.Get("pc-01")
			Expect(r.Hostname).To(Equal("renamed-host"))
			Expect(r.Load5).To(Equal(1.23))
		})

		It("returns false for an unknown client", func() {
			s := newStore()
			_, ok := s.Get("does-not-exist")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("All", func() {
		It("marks a freshly registered client as online", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))

			clients := s.All()
			Expect(clients).To(HaveLen(1))
			Expect(clients[0].Online).To(BeTrue())
		})

		It("returns an empty list when no clients are registered", func() {
			s := newStore()
			Expect(s.All()).To(BeEmpty())
		})
	})

	Describe("Task queue", func() {
		It("returns enqueued tasks via PopTasks", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))
			task := api.Task{ID: "task-1", Type: "exec", Payload: map[string]string{"command": "hostname"}}
			s.AddTask("pc-01", task)

			tasks := s.PopTasks("pc-01")
			Expect(tasks).To(HaveLen(1))
			Expect(tasks[0].ID).To(Equal("task-1"))
		})

		It("empties the queue after PopTasks", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))
			s.AddTask("pc-01", api.Task{ID: "task-1"})
			s.PopTasks("pc-01")

			Expect(s.PopTasks("pc-01")).To(BeEmpty())
		})

		It("returns nothing for an unknown client", func() {
			s := newStore()
			Expect(s.PopTasks("nobody")).To(BeEmpty())
		})

		It("queues multiple tasks in order", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))
			s.AddTask("pc-01", api.Task{ID: "task-1"})
			s.AddTask("pc-01", api.Task{ID: "task-2"})

			tasks := s.PopTasks("pc-01")
			Expect(tasks).To(HaveLen(2))
			Expect(tasks[0].ID).To(Equal("task-1"))
			Expect(tasks[1].ID).To(Equal("task-2"))
		})
	})

	Describe("Task results", func() {
		It("stores and retrieves a task result", func() {
			s := newStore()
			s.SaveTaskResult(api.TaskResult{
				TaskID:   "task-1",
				ClientID: "pc-01",
				Output:   "hello",
				ExitCode: 0,
			})

			result, err := s.GetTaskResult("task-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Output).To(Equal("hello"))
			Expect(result.ExitCode).To(Equal(0))
		})

		It("returns an error for a missing task result", func() {
			s := newStore()
			_, err := s.GetTaskResult("does-not-exist")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Long-poll notifications", func() {
		It("notifies a subscriber when a task is added", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))

			ch := s.Subscribe("pc-01")
			go func() {
				time.Sleep(10 * time.Millisecond)
				s.AddTask("pc-01", api.Task{ID: "task-1"})
			}()

			Eventually(ch, "500ms").Should(Receive())
		})

		It("does not notify after Unsubscribe", func() {
			s := newStore()
			s.Upsert(heartbeat("pc-01"))

			ch := s.Subscribe("pc-01")
			s.Unsubscribe("pc-01")
			s.AddTask("pc-01", api.Task{ID: "task-1"})

			Consistently(ch, "50ms").ShouldNot(Receive())
		})
	})

	Describe("File persistence", func() {
		It("persists clients to disk and reloads them", func() {
			f, err := os.CreateTemp("", "rmm-store-*.json")
			Expect(err).NotTo(HaveOccurred())
			f.Close()
			DeferCleanup(os.Remove, f.Name())

			s1, err := store.New(f.Name())
			Expect(err).NotTo(HaveOccurred())
			s1.Upsert(heartbeat("pc-01"))

			s2, err := store.New(f.Name())
			Expect(err).NotTo(HaveOccurred())
			r, ok := s2.Get("pc-01")
			Expect(ok).To(BeTrue())
			Expect(r.Hostname).To(Equal("pc-01-host"))
		})
	})
})
