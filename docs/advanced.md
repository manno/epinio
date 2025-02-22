# Epinio, Advanced Topics

Opinionated platform that runs on Kubernetes, that takes you from App to URL in one step.

![CI](https://github.com/epinio/epinio/workflows/CI/badge.svg)

<img src="./docs/epinio.png" width="50%" height="50%">

## Contents

- [Traefik](#traefik)
- [Linkerd](#linkerd)
- [Traefik and Linkerd](#traefik-and-linkerd)

## Traefik

When you installed Epinio, it looked at your cluster to see if you had
[Traefik](https://doc.traefik.io/traefik/providers/kubernetes-ingress/)
running. If it wasn't there it installed it.

As Epinio only check two namespaces for Traefik's presence, namely
`traefik` and `kube-system`, it is possible that it tries to install
it, despite the cluster having Traefik running. Just in an unexpected
place.

The `install` command provides the option `--skip-traefik` to handle
this kind of situation.

Installing Epinio on your cluster with the command

```bash
$ epinio install --skip-traefik
```

forces Epinio to not install its own Traefik.

Note that having some other (non-Traefik) Ingress controller running
is __not__ a reason to prevent Epinio from installing Traefik. All the
Ingresses used by Epinio expect to be handled by Traefik.

## Linkerd

By default, Epinio installs [Linkerd](https://linkerd.io/) on your cluster. The
various namespaces created by Epinio become part of the Linkerd Service Mesh and
thus all communication between pods is secured with mutualTLS.

In some cases you may not want Epinio to install Linkerd, either because you did
that manually before you install Epinio or for other reasons. You can provide
the `--skip-linkerd` flag to the `install` command to prevent Epinio from
installing any of the Linkerd control plane components:

```bash
$ epinio install --skip-linkerd
```

## Traefik and Linkerd

By default, with Epinio installing both Traefik and Linkerd, Epinio's
installation process ensures that the Traefik pods are included in the Linkerd
mesh, thus ensuring that external communication to applications is secured on
the leg between loadbalancer and application service.

__However__, there are situations where Epinio does not install Traefik.
This can be because the user specified `--skip-traefik`, or because Epinio
detected a running Traefik, thus can forego its own.
The latter is possible, for example, when using `k3d` as cluster foundation.

In these situations the pre-existing Traefik is __not__ part of the Linkerd
mesh. As a consequence the communication from loadbalancer to application
service is not as secure.

While it is possible to fix this, the fix requires access to the cluster in
general, and to Traefik's namespace specifically. In other words, permissions to
annotate the Traefik namespace are needed, as well as permissions to restart the
pods in that namespace. The latter is necessary, because Linkerd is not able to
inject itself into running pods. It can only intercept pod (re)starts.

### Example

Assuming that Traefik's namespace is called `traefik`, with pods
`traefik-6f9cbd9bd4-z4zw8` and `svclb-traefik-q8g75`
the commands

```
kubectl annotate namespace     traefik linkerd.io/inject=enabled
kubectl delete pod --namespace traefik traefik-6f9cbd9bd4-z4zw8 svclb-traefik-q8g75
```

will bring the Traefik in that namespace into Epinio's linkerd mesh.

Note that this recipe also works for a Traefik provided by `k3d`, in the
`kube-system` namespace.

While it is the namespace which is annotated, only restarted pods are affected
by that, i.e. Traefik's pods here. The other system pods continue to run as they
are.
