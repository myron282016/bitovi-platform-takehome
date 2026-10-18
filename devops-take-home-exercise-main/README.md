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
