//go:build e2e

package e2e_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"notebook/test/e2e/internal"
)

var state internal.State

// envCreatedBySuite records whether THIS suite run is the one that
// brought the environment up, as opposed to reusing one a separate CI
// step (`devcluster up`) already created. Without this, AfterSuite's
// unconditional Down() deletes the environment the very next CI step
// (the Playwright suite) was about to use — Down() always runs after
// specs, pass or fail, so this isn't a hang/leak bug visible only on
// failure; it breaks the normal-success CI path every time. Calling
// Up() unconditionally here was also its own bug independent of
// teardown: it re-ran the full install/secret/port-forward sequence
// on top of an already-up environment, which is exactly the "up run
// twice without a down" scenario the plan's own Review Focus calls
// out — reusing the existing state outright avoids that case entirely
// for this path, rather than merely tolerating it.
var envCreatedBySuite bool

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notebook E2E Suite")
}

var _ = BeforeSuite(func() {
	if loaded, err := internal.LoadState(); err == nil {
		state = loaded
		envCreatedBySuite = false
		return
	}
	envCreatedBySuite = true
	Expect(internal.Up(context.Background())).To(Succeed())
	var err error
	state, err = internal.LoadState()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if envCreatedBySuite {
		Expect(internal.Down(context.Background(), false)).To(Succeed())
	}
})
