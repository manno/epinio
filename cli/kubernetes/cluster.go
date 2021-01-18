package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"

	generic "github.com/suse/carrier/cli/kubernetes/platform/generic"
	ibm "github.com/suse/carrier/cli/kubernetes/platform/ibm"
	k3s "github.com/suse/carrier/cli/kubernetes/platform/k3s"
	kind "github.com/suse/carrier/cli/kubernetes/platform/kind"
	minikube "github.com/suse/carrier/cli/kubernetes/platform/minikube"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	// https://github.com/kubernetes/client-go/issues/345
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type Platform interface {
	Detect(*kubernetes.Clientset) bool
	Describe() string
	String() string
	Load(*kubernetes.Clientset) error
	ExternalIPs() []string
}

var SupportedPlatforms []Platform = []Platform{
	kind.NewPlatform(),
	k3s.NewPlatform(),
	ibm.NewPlatform(),
	minikube.NewPlatform(),
}

type Cluster struct {
	//	InternalIPs []string
	//	Ingress     bool
	Kubectl    *kubernetes.Clientset
	RestConfig *restclient.Config
	platform   Platform
}

// NewClusterFromClient creates a new Cluster from a Kubernetes rest client config
func NewClusterFromClient(restConfig *restclient.Config) (*Cluster, error) {
	c := &Cluster{}

	c.RestConfig = restConfig
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	c.Kubectl = clientset
	c.detectPlatform()
	if c.platform == nil {
		c.platform = generic.NewPlatform()
	}

	return c, c.platform.Load(clientset)
}

func NewCluster(kubeconfig string) (*Cluster, error) {
	c := &Cluster{}
	return c, c.Connect(kubeconfig)
}

func (c *Cluster) GetPlatform() Platform {
	return c.platform
}

func (c *Cluster) Connect(config string) error {
	restConfig, err := clientcmd.BuildConfigFromFlags("", config)
	if err != nil {
		return err
	}
	c.RestConfig = restConfig
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	c.Kubectl = clientset
	c.detectPlatform()
	if c.platform == nil {
		c.platform = generic.NewPlatform()
	}

	err = c.platform.Load(clientset)
	if err == nil {
		fmt.Println(c.platform.Describe())
	}
	return err
}

func (c *Cluster) detectPlatform() {
	for _, p := range SupportedPlatforms {
		if p.Detect(c.Kubectl) {
			c.platform = p
			return
		}
	}
}

// return a condition function that indicates whether the given pod is
// currently running
func (c *Cluster) IsPodRunning(podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.Kubectl.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, cont := range pod.Status.ContainerStatuses {
			if cont.State.Waiting != nil {
				//fmt.Println("containers still in waiting")
				return false, nil
			}
		}

		for _, cont := range pod.Status.InitContainerStatuses {
			if cont.State.Waiting != nil || cont.State.Running != nil {
				return false, nil
			}
		}

		switch pod.Status.Phase {
		case v1.PodRunning, v1.PodSucceeded:
			return true, nil
		case v1.PodFailed:
			return false, nil
		}
		return false, nil
	}
}

func (c *Cluster) PodExists(namespace, selector string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := c.ListPods(namespace, selector)
		if err != nil {
			return false, err
		}
		if len(podList.Items) == 0 {
			return false, nil
		}
		return true, nil
	}
}

// Poll up to timeout seconds for pod to enter running state.
// Returns an error if the pod never enters the running state.
func (c *Cluster) WaitForPodRunning(namespace, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, c.IsPodRunning(podName, namespace))
}

// ListPods returns the list of currently scheduled or running pods in `namespace` with the given selector
func (c *Cluster) ListPods(namespace, selector string) (*v1.PodList, error) {
	listOptions := metav1.ListOptions{}
	if len(selector) > 0 {
		listOptions.LabelSelector = selector
	}
	podList, err := c.Kubectl.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

// Wait up to timeout seconds for all pods in 'namespace' with given 'selector' to enter running state.
// Returns an error if no pods are found or not all discovered pods enter running state.
func (c *Cluster) WaitUntilPodBySelectorExist(namespace, selector string, timeout int) error {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond) // Build our new spinner
	s.Suffix = emoji.Sprintf(" Waiting for resource %s to be created in %s ... :zzz: ", selector, namespace)
	s.Start() // Start the spinner
	defer s.Stop()
	return wait.PollImmediate(time.Second, time.Duration(timeout)*time.Second, c.PodExists(namespace, selector))
}

// WaitForPodBySelectorRunning waits timeout seconds for all pods in 'namespace'
// with given 'selector' to enter running state. Returns an error if no pods are
// found or not all discovered pods enter running state.
func (c *Cluster) WaitForPodBySelectorRunning(namespace, selector string, timeout int) error {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond) // Build our new spinner
	s.Suffix = emoji.Sprintf(" Waiting for resource %s to be running in %s ... :zzz: ", selector, namespace)
	s.Start() // Start the spinner
	defer s.Stop()
	podList, err := c.ListPods(namespace, selector)
	if err != nil {
		return errors.Wrapf(err, "failed listingpods with selector %s", selector)
	}

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods in %s with selector %s", namespace, selector)
	}

	for _, pod := range podList.Items {
		s.Stop()
		s.Suffix = emoji.Sprintf(" Waiting for pod %s to be running in %s ... :zzz: ", pod.Name, namespace)
		s.Start()
		if err := c.WaitForPodRunning(namespace, pod.Name, time.Duration(timeout)*time.Second); err != nil {
			return errors.Wrapf(err, "failed waiting for %s", pod.Name)
		}
	}
	return nil
}

func (c *Cluster) Exec(namespace, podName, containerName string, command, stdin string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	stdinput := bytes.NewBuffer([]byte(stdin))

	err := c.execPod(namespace, podName, containerName, command, stdinput, &stdout, &stderr)

	// if options.PreserveWhitespace {
	// 	return stdout.String(), stderr.String(), err
	// }
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// GetSecret gets a secret's values
func (c *Cluster) GetSecret(namespace, name string) (*v1.Secret, error) {
	secret, err := c.Kubectl.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	return secret, nil
}

// GetVersion get the kube server version
func (c *Cluster) GetVersion() (string, error) {
	v, err := c.Kubectl.ServerVersion()
	if err != nil {
		return "", errors.Wrap(err, "failed to get kube server version")
	}

	return v.String(), nil
}

// ListIngress returns the list of available ingresses in `namespace` with the given selector
func (c *Cluster) ListIngress(namespace, selector string) (*networkingv1.IngressList, error) {
	listOptions := metav1.ListOptions{}
	if len(selector) > 0 {
		listOptions.LabelSelector = selector
	}
	ingressList, err := c.Kubectl.NetworkingV1().Ingresses(namespace).List(context.Background(), listOptions)
	if err != nil {
		return nil, err
	}
	return ingressList, nil
}

func (c *Cluster) execPod(namespace, podName, containerName string,
	command string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := []string{
		"sh",
		"-c",
		command,
	}
	req := c.Kubectl.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}
	if stdin == nil {
		option.Stdin = false
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(c.RestConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}

	return nil
}
