# Bitovi Platform Take-Home: Harden a Service for Production

**Time budget: 2–3 hours. Please don't exceed it.** We'd rather see a smaller
scope done well with clear reasoning than everything half-finished. If you run
out of time, note what you'd do next in `DESIGN.md`.

## The situation

A teammate wrote a small Go worker (`service/`) and a first-pass Helm chart
(`charts/`). It builds, it deploys, and it serves traffic — but it is not
production-ready. You own this now.

You do **not** need to write Go. The service is done and its behavior is
documented in `service/main.go` — read it. Your job is the platform layer.

## What we want you to do

**1. Container.** Make `service/Dockerfile` production-grade. We should be able
to `docker build` it and get a small, sensible image.

**2. Kubernetes / Helm.** Harden the chart in `charts/` so
it's fit to run in production. We should be able to install it into a local
`kind`/`minikube` cluster. Use your judgment about what "production-ready"
means for a workload like this — the current chart has several things a
reviewer would flag.

**3. GitOps.** This repo is going to be synced by Argo CD (or Flux — your
choice) with no manual `kubectl apply`. Add whatever's needed to make that
work, and structure the repo accordingly. A plaintext secret committed to a
Git repo that a GitOps controller syncs is a problem — handle it.

As a starting point, a common
Argo CD layout is:

```
charts/
argocd/
  application.yaml
```

**4. Observability.** The service exposes Prometheus metrics at `/metrics`.
Make sure they'd actually get scraped in a cluster running the Prometheus
Operator.

**5. `DESIGN.md`.** Short written answers (a paragraph or two each — bullets
are fine):

- **Idempotency.** This worker consumes order events and will sometimes receive
  the same event more than once. How do you make processing idempotent, and
  where does that responsibility live — the app, the platform, or both?
- **Scaling & backpressure.** Traffic is spiky. How would you scale this safely?
  What breaks first, and how do you keep the worker from overwhelming its
  downstream Postgres when it does?
- **Breaking rollout.** You need to ship a change to the order event schema
  that is **not** backward-compatible, with zero downtime and no dropped or
  double-processed events. Walk us through the rollout.

## Deliverables

- The modified repo (a Git repo with readable commit history is a plus).
- `DESIGN.md` with the three answers above and a short note on anything you'd
  do with more time.
- Update this section of the README (or add `NOTES.md`) with the commands to
  build and deploy, and a sentence on any tradeoffs or assumptions you made.

## What we're evaluating

Judgment and reasoning over checklist completion. We care that you understand
_why_ each change matters, not that you found every last nit. A tidy, well-
reasoned submission that's honest about its tradeoffs beats an exhaustive one
that can't explain itself.



## Implementation, Deployment & Validation
### Overview

This project hardens the provided Go order-processing worker for containerized deployment using Docker, Kubernetes, Helm, and Argo CD.

The implementation focuses on security, reliability, graceful shutdown, GitOps, and observability.

The original Go service was not modified. It simulates background order processing rather than consuming actual queue messages or connecting to PostgreSQL.

### 1. Implemented Improvements

**Containerization**
- Multi-stage Docker build with a minimal distroless runtime image.
- Non-root container execution.
- Reduced final image size and unnecessary runtime dependencies.
- `.dockerignore` to reduce the Docker build context.

**Kubernetes / Helm**
- Separate liveness (`/healthz`) and readiness (`/readyz`) probes.
- Startup probe for initialization.
- 30-second termination grace period to accommodate the worker's 20-second drain timeout.
- CPU and memory resource requests and limits.
- Restricted container security context and read-only root filesystem.
- Rolling deployment strategy.
- Kubernetes Secret references instead of plaintext database credentials.

**GitOps**
- Argo CD Application configuration targeting the Helm chart in GitHub.
- Automated synchronization, pruning, and self-healing.
- Validated a GitHub configuration change triggering a Kubernetes reconciliation.

**Observability**
- Prometheus `ServiceMonitor` template targeting `/metrics`.
- Configurable monitoring enablement and Prometheus release label.
- Exposed metrics for processed orders, in-flight orders, and process uptime.

### 2. Prerequisites

Local development and deployment require:

- Docker Desktop with a running Docker engine.
- Kubernetes cluster (Docker Desktop Kubernetes, kind, or minikube).
- `kubectl` configured to communicate with the cluster.
- Helm 3.
- Git.

Optional integrations:

- Argo CD for GitOps synchronization.
- Prometheus Operator and Prometheus for metrics collection.
- External Secrets Operator with a supported secret provider for production credential management.

Verify the local tools:

```powershell
docker --version
kubectl version --client
helm version
kubectl get nodes
```

The Kubernetes node should report `Ready`.

### 3. Build the Docker Image

From the repository root:

```powershell
docker build -t order-processor:v1 ./service
```

Inspect the image:

```powershell
docker images order-processor
```

Run the container locally:

```powershell
docker run --rm -d --name order-processor -p 8080:8080 order-processor:v1
```

Test the HTTP endpoints:

```powershell
curl.exe -i http://localhost:8080/healthz
curl.exe -i http://localhost:8080/readyz
curl.exe http://localhost:8080/metrics
```

Expected behavior:

- `/healthz`: HTTP 200 while the server is healthy.
- `/readyz`: HTTP 503 during the initial five-second warmup, then HTTP 200.
- `/metrics`: Prometheus-compatible text metrics.

Stop the container:

```powershell
docker stop -t 30 order-processor
```

The image must be accessible to the Kubernetes container runtime. Docker Desktop may make locally built images available to its Kubernetes cluster, depending on cluster configuration. For kind or minikube, load the image using the appropriate image-loading command.

### 4. Kubernetes Secret Configuration

The Helm chart references an existing Kubernetes Secret rather than storing the database password in Git.

Example Helm values:

```yaml
db:
  host: orders-db.internal
  existingSecret: order-processor-db
  passwordKey: password
```

The Secret must exist in the same namespace as the worker.

For a local demonstration, create it in the default namespace:

```powershell
kubectl create secret generic order-processor-db `
  --from-literal=password="local-demo-only-password"
```

For the Argo CD test namespace, provision a separate demonstration Secret:

```powershell
kubectl create secret generic order-processor-db `
  -n bitovi-gitops-test `
  --from-literal=password="local-demo-only-password"
```

The literal password above is for local testing only. Do not use real credentials on the command line or commit them to Git. Production secrets should be provisioned through an approved secure mechanism.

Verify the Secret without displaying its value:

```powershell
kubectl describe secret order-processor-db -n default
```

The output should show a `password` data key.

**Production recommendation:** Integrate External Secrets Operator with AWS Secrets Manager or another approved secrets provider. The operator can securely synchronize credentials into namespace-scoped Kubernetes Secrets using workload identity and appropriately scoped permissions.

The Go worker does not currently establish a PostgreSQL connection; this exercise validates the secret-injection configuration rather than database connectivity.

### 5. Validate the Helm Chart

From the repository root:

```powershell
helm lint ./charts
```

Render the Kubernetes manifests:

```powershell
helm template order-processor ./charts
```

These commands validate the chart and render manifests without deploying resources.

### 6. Deploy Using Helm

For local testing in the `default` namespace:

```powershell
helm upgrade --install order-processor ./charts `
  --namespace default `
  --set monitoring.enabled=false `
  --wait `
  --timeout 2m
```

Monitoring is disabled because the local Kubernetes cluster does not have the Prometheus Operator `ServiceMonitor` CRD installed.

Verify the Deployment:

```powershell
kubectl get deployments
kubectl get pods
kubectl get services
```

Check rollout status:

```powershell
kubectl rollout status deployment/order-processor-order-processor
```

Inspect Kubernetes configuration and events:

```powershell
kubectl describe deployment order-processor-order-processor
kubectl describe pod -l app=order-processor
```

Expected result: one running, ready worker replica without persistent health-check errors.

**Note:** This direct Helm installation is for local validation. Once Argo CD manages a deployment, routine updates should be made through Git rather than by running competing manual Helm upgrades against the same resources.

### 7. Test the Application in Kubernetes

Forward the Kubernetes Service to the local machine:

```powershell
kubectl port-forward svc/order-processor-order-processor 8080:80
```

In another terminal:

```powershell
curl.exe -i http://localhost:8080/healthz
curl.exe -i http://localhost:8080/readyz
curl.exe http://localhost:8080/metrics
```

The application exposes these metrics:

- `orders_processed_total`
- `orders_inflight`
- `process_uptime_seconds`

`orders_processed_total` increases because the application periodically simulates processing. It does not represent real customer orders and resets on process restart.

### 8. Argo CD GitOps Deployment

The repository includes:

```text
argocd/
  application.yaml
```

The Application references the GitHub repository, the `master` branch, and the Helm chart path.

The configured deployment namespace is `bitovi-gitops-test`.

#### Install Argo CD (if not already installed)

```powershell
kubectl create namespace argocd
```

Install using an approved, pinned Argo CD release for reproducible environments. For a local demonstration, the official stable installation manifest can be used:

```powershell
kubectl apply -n argocd --server-side --force-conflicts `
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

Check the Argo CD components:

```powershell
kubectl get pods -n argocd
```

#### Bootstrap the Application

First ensure that:

- The source repository and chart path are correct.
- The desired image is accessible to the target cluster.
- The required Kubernetes Secret exists in the target namespace.
- `monitoring.enabled` is set to `false` for clusters without Prometheus Operator.

Create the test namespace if it does not exist:

```powershell
kubectl create namespace bitovi-gitops-test
```

Register the Application:

```powershell
kubectl apply -f ./argocd/application.yaml
```

This is a one-time bootstrap operation. Argo CD subsequently reconciles Helm configuration changes from Git automatically.

Verify:

```powershell
kubectl get applications -n argocd
kubectl get pods -n bitovi-gitops-test
```

Expected Application status:

```text
SYNC STATUS   HEALTH STATUS
Synced        Healthy
```

#### Access the Argo CD Dashboard

```powershell
kubectl port-forward svc/argocd-server -n argocd 8081:443
```

Open:

https://localhost:8081

The initial administrator credentials are provided by the local Argo CD installation. Protect them appropriately.

#### GitOps Validation

Automatic synchronization was tested by changing the desired replica count in GitHub and observing Argo CD update Kubernetes.

Example:

```yaml
replicaCount: 2
```

After committing and pushing the change to the monitored branch, verify:

```powershell
kubectl get applications -n argocd
kubectl get deployments -n bitovi-gitops-test
kubectl get pods -n bitovi-gitops-test
```

Restore `replicaCount: 1` after the test.

Argo CD synchronizes Kubernetes resources but does not build or publish container images. Application changes require an image build/publish workflow and an updated image reference in Git.

### 9. Prometheus Observability

The worker exposes metrics at:

```text
/metrics
```

The Helm chart includes a conditional `ServiceMonitor` template for Prometheus Operator-enabled clusters.

Configure:

```yaml
monitoring:
  enabled: true
  prometheusRelease: kube-prometheus-stack
```

The `prometheusRelease` value must match the relevant ServiceMonitor label selector used by the Prometheus installation.

Render the manifest:

```powershell
helm template order-processor ./charts `
  --set monitoring.enabled=true
```

In a cluster with Prometheus Operator CRDs installed, monitoring can be enabled through Helm or the GitOps-managed Helm values.

Verify:

```powershell
kubectl get servicemonitor -A
```

A Prometheus query that can confirm metric collection is:

```promql
orders_processed_total
```

For scrape health, inspect the worker target's `up` metric.

**Local testing limitation:** The Docker Desktop cluster used for this exercise did not have Prometheus Operator installed. The ServiceMonitor template was prepared for an operator-enabled cluster, and the application's `/metrics` endpoint was tested, but end-to-end Prometheus scraping was not verified.

### 10. Design Decisions and Tradeoffs

- **Application scope:** The supplied Go service was intentionally left unchanged. Queue processing and PostgreSQL interaction are simulated.
- **Resource sizing:** CPU and memory settings are initial estimates and should be refined through profiling and load testing.
- **Autoscaling:** No HPA or KEDA autoscaling was deployed. Queue-driven scaling requires a real queue and downstream-capacity controls.
- **Secrets:** Kubernetes Secret injection was verified locally. Production secret synchronization from a cloud provider was not implemented.
- **GitOps:** Argo CD automatic configuration synchronization was tested. Container image publication remains a separate responsibility.
- **Monitoring:** The ServiceMonitor configuration was implemented, but actual scraping requires Prometheus Operator and compatible selectors.
- **Deployment reliability:** Rolling updates and graceful shutdown reduce interruption risk but do not independently guarantee exactly-once business processing.

### 11. Future Improvements

Given additional implementation time, the next priorities would be:

1. Add GitHub Actions to test the service, build and scan container images, and publish immutable versions to GitHub Container Registry or another approved registry.
2. Automate updating the Helm image reference so Argo CD can deploy newly published images.
3. Integrate External Secrets Operator with AWS Secrets Manager or another centralized secrets provider.
4. Validate Prometheus scraping, dashboards, and alerting in an operator-equipped cluster.
5. Introduce KEDA-based scaling for a real queue-consuming worker with bounded concurrency and PostgreSQL connection limits.
6. Implement automated deployment smoke tests, graceful-shutdown validation, load testing, and rollback verification.
7. Pin container base images and deployment dependencies to tested immutable versions.

See `DESIGN.md` for the detailed discussions of idempotency, scaling and backpressure, breaking-schema rollouts, and failure recovery.

### 12. Final Validation

Before submission, run:

```powershell
helm lint ./charts
helm template order-processor ./charts
helm template order-processor ./charts --set monitoring.enabled=true

kubectl get applications -n argocd
kubectl get deployments -n bitovi-gitops-test
kubectl get pods -n bitovi-gitops-test

git status
```

Confirm that:

- Helm validation passes.
- Argo CD reports `Synced` and `Healthy`.
- The worker Pod is Running and Ready.
- The Kubernetes Secret reference is configured without exposing the password.
- `DESIGN.md` contains all three required architecture answers.
- The repository contains no real credentials or unnecessary local development artifacts.

The final submission consists of the modified source repository, `DESIGN.md`, and updated deployment documentation.



### GitOps Validation Results

Successfully configured and tested Argo CD synchronization between GitHub and the local Kubernetes cluster.

**Validation performed:**
- Confirmed Argo CD could access the GitHub repository and render the Helm chart.
- Verified the application reported `Synced` and `Healthy`.
- Updated `replicaCount` from `1` to `2` in `charts/values.yaml` and pushed the change to GitHub.
- Confirmed Argo CD automatically reconciled the Deployment and Kubernetes created the additional worker Pod.
- Restored `replicaCount` to `1` and verified synchronization completed successfully.

**Result:** GitOps synchronization was successfully validated without manually running `helm upgrade` or `kubectl apply` for application configuration changes.

**Limitation:** Container image building and publishing are not automated. A future GitHub Actions pipeline would build and publish versioned images, update the Helm image reference, and allow Argo CD to deploy new application versions.