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

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notebook E2E Suite")
}

var _ = BeforeSuite(func() {
	Expect(internal.Up(context.Background())).To(Succeed())
	var err error
	state, err = internal.LoadState()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Expect(internal.Down(context.Background(), false)).To(Succeed())
})
