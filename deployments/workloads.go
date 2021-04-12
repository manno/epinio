package deployments

import (
	"context"
	"fmt"
	"time"

	"github.com/epinio/epinio/helpers"
	"github.com/epinio/epinio/internal/duration"
	"github.com/epinio/epinio/kubernetes"
	"github.com/epinio/epinio/termui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedbatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
)

type Workloads struct {
	Debug   bool
	Timeout time.Duration
}

const (
	WorkloadsDeploymentID   = "epinio-workloads"
	WorkloadsIngressVersion = "0.1"
	appIngressYamlPath      = "app-ingress.yaml"
)

func (k *Workloads) ID() string {
	return WorkloadsDeploymentID
}

func (k *Workloads) Backup(c *kubernetes.Cluster, ui *termui.UI, d string) error {
	return nil
}

func (k *Workloads) Restore(c *kubernetes.Cluster, ui *termui.UI, d string) error {
	return nil
}

func (k Workloads) Describe() string {
	return emoji.Sprintf(":cloud:Workloads Eirinix Ingress Version: %s\n", WorkloadsIngressVersion)
}

// Delete removes Workloads from kubernetes cluster
func (w Workloads) Delete(c *kubernetes.Cluster, ui *termui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Workloads...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(WorkloadsDeploymentID)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", WorkloadsDeploymentID)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Workspace because namespace either doesn't exist or not owned by Epinio")
		return nil
	}

	if err := w.deleteWorkloadsNamespace(c, ui); err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", WorkloadsDeploymentID)
	}

	existsAndOwned, err = c.NamespaceExistsAndOwned("app-ingress")
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace 'app-ingress' is owned or not")
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping app-ingress namespace deletion because either doesn't exist or not owned by Epinio")
		return nil
	}

	if out, err := helpers.KubectlDeleteEmbeddedYaml(appIngressYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", appIngressYamlPath, out))
	}

	ui.Success().Msg("Workloads removed")

	return nil
}

func (w Workloads) apply(c *kubernetes.Cluster, ui *termui.UI, options kubernetes.InstallationOptions) error {
	if err := w.createWorkloadsNamespace(c, ui); err != nil {
		return err
	}

	if out, err := helpers.KubectlApplyEmbeddedYaml(appIngressYamlPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", appIngressYamlPath, out))
	}

	if err := c.LabelNamespace("app-ingress", kubernetes.EpinioDeploymentLabelKey, kubernetes.EpinioDeploymentLabelValue); err != nil {
		return err
	}

	if err := c.WaitUntilPodBySelectorExist(ui, "app-ingress", "name=app-ingress", w.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting app-ingress deployment to exist")
	}

	ui.Success().Msg("Workloads deployed")

	return nil
}

func (k Workloads) GetVersion() string {
	// TODO: Maybe this should be the Epinio version itself?
	return WorkloadsIngressVersion
}

func (k Workloads) Deploy(c *kubernetes.Cluster, ui *termui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		WorkloadsDeploymentID,
		metav1.GetOptions{},
	)
	if err == nil {
		return errors.New("Namespace " + WorkloadsDeploymentID + " present already")
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Workloads...")

	err = k.apply(c, ui, options)
	if err != nil {
		return err
	}

	s := ui.Progress("Warming up cluster with builder image")
	err = k.warmupBuilder(c)
	if err != nil {
		return err
	}
	s.Stop()

	return nil
}

func (k Workloads) Upgrade(c *kubernetes.Cluster, ui *termui.UI, options kubernetes.InstallationOptions) error {
	// NOTE: Not implemented yet
	return nil
}

func (w Workloads) createWorkloadsNamespace(c *kubernetes.Cluster, ui *termui.UI) error {
	if _, err := c.Kubectl.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
				Labels: map[string]string{
					"quarks.cloudfoundry.org/monitored": "quarks-secret",
					kubernetes.EpinioDeploymentLabelKey: kubernetes.EpinioDeploymentLabelValue,
				},
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		return nil
	}

	if err := c.LabelNamespace(WorkloadsDeploymentID, kubernetes.EpinioDeploymentLabelKey, kubernetes.EpinioDeploymentLabelValue); err != nil {
		return err
	}
	if err := w.createGiteaCredsSecret(c); err != nil {
		return err
	}
	if err := w.createClusterRegistryCredsSecret(c); err != nil {
		return err
	}
	if err := w.createWorkloadsServiceAccountWithSecretAccess(c); err != nil {
		return err
	}

	return nil
}

func (w Workloads) deleteWorkloadsNamespace(c *kubernetes.Cluster, ui *termui.UI) error {
	message := "Deleting Workloads namespace " + WorkloadsDeploymentID
	_, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(WorkloadsDeploymentID)
		},
	)
	if err != nil {
		return err
	}

	message = "Waiting for workloads namespace to be gone"
	warning, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			var err error
			for err == nil {
				_, err = c.Kubectl.CoreV1().Namespaces().Get(
					context.Background(),
					WorkloadsDeploymentID,
					metav1.GetOptions{},
				)
			}
			if serr, ok := err.(*apierrors.StatusError); ok {
				if serr.ErrStatus.Reason == metav1.StatusReasonNotFound {
					return "", nil
				}
			}

			return "", err
		},
	)
	if err != nil {
		return err
	}
	if warning != "" {
		ui.Exclamation().Msg(warning)
	}

	return nil
}

func (w Workloads) createClusterRegistryCredsSecret(c *kubernetes.Cluster) error {
	// TODO: Are all of these really used? We need tekton to be able to access
	// the registry and also kubernetes (when we deploy our app deployments)
	auths := `{ "auths": {
		"https://127.0.0.1:30500":{"auth": "YWRtaW46cGFzc3dvcmQ=", "username":"admin","password":"password"},
		"http://127.0.0.1:30501":{"auth": "YWRtaW46cGFzc3dvcmQ=", "username":"admin","password":"password"},
		 "registry.epinio-registry":{"username":"admin","password":"password"},
		 "registry.epinio-registry:444":{"username":"admin","password":"password"} } }`

	_, err := c.Kubectl.CoreV1().Secrets(WorkloadsDeploymentID).Create(context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "registry-creds",
			},
			StringData: map[string]string{
				".dockerconfigjson": auths,
			},
			Type: "kubernetes.io/dockerconfigjson",
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

func (w Workloads) createGiteaCredsSecret(c *kubernetes.Cluster) error {
	_, err := c.Kubectl.CoreV1().Secrets(WorkloadsDeploymentID).Create(context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gitea-creds",
				Annotations: map[string]string{
					//"kpack.io/git": fmt.Sprintf("http://%s.%s", GiteaDeploymentID, domain),
					"tekton.dev/git-0": "http://gitea-http.gitea:10080", // TODO: Don't hardcode
				},
			},
			StringData: map[string]string{
				"username": "dev",
				"password": "changeme",
			},
			Type: "kubernetes.io/basic-auth",
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

// Adding the imagePullSecrets to the service account attached to the application
// pods, will automatically assign the same imagePullSecrets to the pods themselves:
// https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#verify-imagepullsecrets-was-added-to-pod-spec
func (w Workloads) createWorkloadsServiceAccountWithSecretAccess(c *kubernetes.Cluster) error {
	automountServiceAccountToken := false
	_, err := c.Kubectl.CoreV1().ServiceAccounts(WorkloadsDeploymentID).Create(
		context.Background(),
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "registry-creds"},
				{Name: "gitea-creds"},
			},
			AutomountServiceAccountToken: &automountServiceAccountToken,
		}, metav1.CreateOptions{})

	return err
}

// This function creates a dummy Job using the buildpack builder image
// in order to avoid pulling it the first time we an application is staged.
func (w Workloads) warmupBuilder(c *kubernetes.Cluster) error {
	client, err := typedbatchv1.NewForConfig(c.RestConfig)
	if err != nil {
		return err
	}

	jobName := "buildpack-builder-warmup"
	var backoffLimit = int32(1)
	if _, err = client.Jobs(WorkloadsDeploymentID).Create(
		context.Background(),
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: jobName,
				Labels: map[string]string{
					kubernetes.EpinioDeploymentLabelKey: kubernetes.EpinioDeploymentLabelValue,
				},
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &backoffLimit,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "warmup",
								Image:   "quay.io/asgardtech/paketobuildpacks-builder:full-cf", // TODO: DRY this
								Command: []string{"/bin/ls"},
							}},
						RestartPolicy: "Never",
					}}}},
		metav1.CreateOptions{},
	); err != nil {
		return err
	}

	return c.WaitForJobCompleted(WorkloadsDeploymentID, jobName, duration.ToWarmupJobReady())
}
