// Package interfaces defines the various interfaces needed by Epinio.
// e.g. Service, Application etc
package interfaces

import (
	corev1 "k8s.io/api/core/v1"
)

type Service interface {
	Name() string
	Org() string
	GetBinding(appName string) (*corev1.Secret, error)
	DeleteBinding(appName string) error
	Delete() error
	Status() (string, error)
	Details() (map[string]string, error)
	WaitForProvision() error
}

type ServiceList []Service
