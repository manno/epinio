package acceptance_test

import (
	"os"

	"github.com/epinio/epinio/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Epinio enable/disable features", func() {
	Describe("services-incluster", func() {
		BeforeEach(func() {
			// Make sure they are not already enabled
			out, err := Epinio("disable services-incluster", "")
			Expect(err).ToNot(HaveOccurred(), out)
		})

		It("enables minibroker services", func() {
			setupInClusterServices()

			out, err := helpers.Kubectl(`get pods -n minibroker --selector=app=minibroker-minibroker`)
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp(`minibroker.*1/1.*Running`))
		})
	})

	Describe("services-google", func() {
		var serviceAccountJson string
		var err error

		BeforeEach(func() {
			// Make sure they aren't already enabled
			out, err := Epinio("disable services-google", "")
			Expect(err).ToNot(HaveOccurred(), out)

			serviceAccountJson, err = helpers.CreateTmpFile(`
				{
					"type": "service_account",
					"project_id": "myproject",
					"private_key_id": "somekeyid",
					"private_key": "someprivatekey",
					"client_email": "client@example.com",
					"client_id": "clientid",
					"auth_uri": "https://accounts.google.com/o/oauth2/auth",
					"token_uri": "https://oauth2.googleapis.com/token",
					"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
					"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/client%40example.com"
				}
			`)
			Expect(err).ToNot(HaveOccurred(), serviceAccountJson)
		})

		AfterEach(func() {
			err = os.Remove(serviceAccountJson)
			Expect(err).ToNot(HaveOccurred())
		})

		It("enables google services", func() {
			out, err := Epinio("enable services-google --service-account-json "+serviceAccountJson, "")
			Expect(err).ToNot(HaveOccurred(), out)

			out, err = helpers.Kubectl(`get pods -n google-service-broker --selector=app.kubernetes.io/name=gcp-service-broker`)
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp(`google-service-broker-gcp-service-broker.*1/1.*Running`))
		})
	})
})
