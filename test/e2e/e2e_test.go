//go:build e2e

package e2e_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"notebook/test/e2e/internal"
)

var _ = Describe("Notebook app", func() {
	It("serves /healthz", func() {
		resp, err := http.Get(state.BaseURL + "/healthz")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("redirects /auth/login to the Dex authorization endpoint", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err := client.Get(state.BaseURL + "/auth/login")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring(state.DexIssuer))
	})

	It("lets an admin log in via Dex and create a note that appears in the list", func() {
		client := internal.NewCookieClient(state.DexLocalAddr)
		Expect(internal.LoginViaDex(client, state.BaseURL, internal.TestAdminEmail, internal.TestPassword)).To(Succeed())

		created, err := internal.CreateNote(client, state.BaseURL, "E2E Note", "Hello from e2e")
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Title).To(Equal("E2E Note"))

		notes, err := internal.ListNotes(client, state.BaseURL)
		Expect(err).NotTo(HaveOccurred())
		var titles []string
		for _, n := range notes {
			titles = append(titles, n.Title)
		}
		Expect(titles).To(ContainElement("E2E Note"))
	})
})
