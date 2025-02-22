package acceptance_test

import (
	"fmt"

	"github.com/epinio/epinio/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Catalog Services", func() {
	var org string
	var serviceName string
	BeforeEach(func() {
		org = newOrgName()
		serviceName = newServiceName()
		setupAndTargetOrg(org)
	})

	Describe("service create", func() {
		It("creates a catalog based service, with waiting", func() {
			makeCatalogService(serviceName)
		})

		It("creates a catalog based service, with additional data", func() {
			makeCatalogService(serviceName, `{ "db": { "name": "wordpress" }}`)
			serviceInstanceName := fmt.Sprintf("service.org-%s.svc-%s", org, serviceName)

			out, err := helpers.Kubectl(
				fmt.Sprintf("get serviceinstance -n %s %s -o=jsonpath='{.status.externalProperties.parameters.db.name}'",
					org, serviceInstanceName))
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(Equal("wordpress"))
		})

		It("creates a catalog based service, without waiting", func() {
			makeCatalogServiceDontWait(serviceName)
		})

		AfterEach(func() {
			cleanupService(serviceName)
		})
	})

	Describe("service delete", func() {
		BeforeEach(func() {
			makeCatalogService(serviceName)
		})

		It("deletes a catalog based service", func() {
			deleteService(serviceName)
		})

		It("doesn't delete a bound service", func() {
			appName := newAppName()
			makeApp(appName, 1, true)
			bindAppService(appName, serviceName, org)

			out, err := Epinio("service delete "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("Unable to delete service. It is still used by"))
			Expect(out).To(MatchRegexp(appName))
			Expect(out).To(MatchRegexp("Use --unbind to force the issue"))

			verifyAppServiceBound(appName, serviceName, org, 1)

			// Delete again, and force unbind

			out, err = Epinio("service delete --unbind "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("PREVIOUSLY BOUND TO"))
			Expect(out).To(MatchRegexp(appName))

			Expect(out).To(MatchRegexp("Service Removed"))

			verifyAppServiceNotbound(appName, serviceName, org, 1)

			// And check non-presence
			Eventually(func() string {
				out, err = Epinio("service list", "")
				Expect(err).ToNot(HaveOccurred(), out)
				return out
			}, "10m").ShouldNot(MatchRegexp(serviceName))
		})
	})

	Describe("service bind", func() {
		var appName string
		BeforeEach(func() {
			appName = newAppName()

			makeCatalogService(serviceName)
			makeApp(appName, 1, true)
		})

		AfterEach(func() {
			cleanupApp(appName)
			cleanupService(serviceName)
		})

		It("binds a service to the application deployment", func() {
			bindAppService(appName, serviceName, org)
		})
	})

	Describe("service unbind", func() {
		var appName string
		BeforeEach(func() {
			appName = newAppName()

			makeCatalogService(serviceName)
			makeApp(appName, 1, true)
			bindAppService(appName, serviceName, org)
		})

		AfterEach(func() {
			cleanupApp(appName)
			cleanupService(serviceName)
		})

		It("unbinds a service from the application deployment", func() {
			unbindAppService(appName, serviceName, org)
		})
	})

	Describe("service show", func() {
		BeforeEach(func() {
			makeCatalogService(serviceName)
		})

		It("it shows service details", func() {
			out, err := Epinio("service show "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp("Service Details"))
			Expect(out).To(MatchRegexp(`Status .*\|.* Provisioned`))
			Expect(out).To(MatchRegexp(`Class .*\|.* mariadb`))
			Expect(out).To(MatchRegexp(`Plan .*\|.* 10-3-22`))
		})

		AfterEach(func() {
			cleanupService(serviceName)
		})
	})
})
