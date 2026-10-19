# Design Decisions — Order Processor

## Scope and assumptions

The supplied Go service is a platform exercise, **not a real queue consumer**. Its background loop simulates a completed order about every 200 ms; it does not read order events or connect to PostgreSQL. It exposes `/healthz`, `/readyz`, and `/metrics`, warms up for five seconds, and on SIGTERM marks itself unready and drains in-flight work for up to 20 seconds. The Helm chart, container, GitOps configuration, and monitoring integration are the implementation scope. The event-processing designs below are **proposals for a real production worker**, not implemented application features.

## 1. Idempotency: duplicate order events

**Direct answer — idempotency is primarily the responsibility of the application and database, not Kubernetes.** The application understands an order event's business meaning and whether repeating it would create an incorrect effect. PostgreSQL enforces uniqueness and transactionality. The platform manages Pod health, restarts, scheduling, and scaling; it cannot decide whether an order was already processed. The message broker provides acknowledgements and redelivery but may deliver the same event more than once.

**Implementation:** Give each event a stable event ID. Within one PostgreSQL transaction, atomically insert the event ID into a `processed_events` table (protected by a unique constraint) and apply the order state change. If the ID already exists, avoid repeating the business effect. A unique constraint plus `INSERT ... ON CONFLICT DO NOTHING` is safer under concurrent workers than checking with `SELECT` first. Apply business-level constraints too, since two different event IDs could still represent the same order operation.

**Failure handling:** Acknowledge a message only after the database transaction commits. If the worker crashes before commit, it can safely retry; if it commits but crashes before acknowledging, the broker may redeliver the event and the application detects the duplicate. External effects, such as payments, additionally require downstream idempotency keys or a transactional outbox because the local database transaction does not encompass a third-party API. This is an application-level design proposal; the supplied worker does not actually consume a broker or write to PostgreSQL.

## 2. Scaling and backpressure

**Direct answer — autoscale cautiously based on workload demand and protect the downstream database with hard capacity limits.** For a real queue consumer, use **KEDA** to scale replicas based on queue depth and message age (or HPA with suitable custom metrics). Set minimum and maximum replica counts based on load tests; CPU alone may not reflect queue pressure. Scale down safely by draining in-flight work or allowing unacknowledged messages to be redelivered. The supplied sample has no queue, so KEDA is a proposed production improvement, not currently deployed.

**What breaks first?** PostgreSQL is a **likely bottleneck**, not a certainty. More worker replicas generate more simultaneous connections, inserts, and updates, potentially exhausting connection slots, increasing lock contention, or slowing queries. Other bottlenecks could be the message broker or an external service; use measurements to determine the actual limit. Cap per-Pod processing concurrency and connection-pool size so `maximum replicas × connections per replica` stays within the database connection budget, leaving capacity for other clients.

**Backpressure and recovery:** When the database slows or becomes unavailable, stop or reduce message intake, keep work in the durable queue, use request timeouts and bounded retries with exponential backoff and jitter, and only acknowledge after durable processing. Route permanently invalid messages to a dead-letter queue for controlled replay. Monitor queue lag, processing latency, error/retry rates, and database connections. Autoscaling increases potential throughput; backpressure prevents that throughput from overwhelming downstream services.

## 3. Breaking event-schema rollout with zero downtime

A non-backward-compatible schema must **not** be introduced by simply replacing every producer and consumer at once. Introduce a versioned event contract (for example, `order.v1` and `order.v2` with a schema/version identifier). First deploy consumers capable of handling both versions, or run v1 and v2 consumers on separate versioned topics/queues; deploy compatibility adapters if the existing consumer cannot support both. Verify idempotency and routing across versions, and keep both processing paths available during the transition. Only then gradually switch producers to emit v2 using a controlled rollout. Avoid uncontrolled dual publishing, which can cause double business effects; if temporary dual publish is necessary, preserve a shared logical event identity and deduplicate at the business boundary.

Monitor consumer errors, backlog, event counts, and database invariants through the cutover. Drain and verify all v1 messages (including delayed retries and dead-letter queues) before retiring v1 support. Rollback must be planned **before** v2-only data is persisted: retain compatibility or use a tested translation strategy, since rolling back code alone cannot necessarily undo incompatible data. A zero-downtime/no-loss/no-double-effect objective depends on durable broker retention, careful consumer offsets/acknowledgements, transactional/idempotent side effects, and rehearsed failover tests; a Kubernetes rolling update alone cannot guarantee it.

## Failure scenarios and recovery
| Failure | Detection | Recovery / prevention |
| --- | --- | --- |
| Worker crash or node loss | Pod restarts, unavailable replicas, missing heartbeat/throughput | Kubernetes replaces the Pod; broker redelivers unacknowledged events; database idempotency prevents repeated effects. |
| Database outage or connection exhaustion | DB errors, connection/pool saturation, rising queue age | Pause or throttle consumption; bounded exponential-backoff retries; alert; resume after DB recovery without prematurely acknowledging messages. |
| Duplicate or concurrent delivery | Conflict on event ID, duplicate-rate metrics | Unique constraints and transactional inbox; acknowledge already-committed events as duplicates. |
| Poison or permanently invalid event | Repeated deterministic processing errors | Limited attempts; dead-letter queue with reason and trace ID; validate/fix and replay in a controlled way. |
| Traffic surge | Queue lag, processing latency, DB saturation | Bounded concurrency, backpressure, capped autoscaling and measured connection budgets. |
| Bad rollout / incompatible schema | Readiness or error regression, DLQ spike, data integrity checks | Stop rollout; route to compatible consumer versions; roll back only where data compatibility permits; replay safely. |
| Secret missing or rotated | Pod configuration/creation errors or DB authentication failures | Alert; re-sync from external secret store; roll Pods deliberately when env-var credentials change; never log secret values. |
| Metrics/monitoring unavailable | Missing Prometheus target, scrape errors | Check ServiceMonitor labels, Service port and Operator CRDs; alert on monitoring gaps and use Kubernetes logs/events during recovery. |

## Further improvements / time-boxed tradeoffs
- **CI/CD:** Add GitHub Actions to test and build a versioned image, scan it, publish to GHCR or another registry, and update the Helm image reference with an immutable digest. Argo CD then reconciles Git changes; it does **not** build images. Avoid mutable `latest` tags and prevent bot-created update loops. The local `order-processor:v1` image is not automatically available to a remote cluster.
- **Secrets:** For production, use AWS Secrets Manager or Vault with External Secrets Operator, a scoped `SecretStore`/`ExternalSecret`, and workload identity (for EKS, Pod Identity/IRSA). Restrict IAM access to required secret ARNs, enable encryption at rest and Kubernetes RBAC, and plan rotation and Pod restart/reload. A manually created local Kubernetes Secret demonstrates injection but does **not** provide declarative secret provisioning on a fresh cluster. A Secret's presence alone does not prove database connectivity.
- **Observability:** The worker exposes Prometheus text metrics, and the chart can render a ServiceMonitor. Actual Prometheus scraping was **not** verified locally because the cluster lacked the Prometheus Operator `ServiceMonitor` CRD. In an operator-equipped cluster, match the Prometheus instance's ServiceMonitor namespace/label selectors, verify target `up == 1`, and add alerts/dashboards. Confirm workloads have operationally useful metrics (queue lag, failures, retries, DB saturation), beyond this demo's counters.
- **Integration and resilience tests:** Add queue/PostgreSQL integration tests, concurrent duplicate-insert tests, shutdown/redelivery tests, load tests, and schema migration/rollback rehearsals. Validate graceful termination under real in-flight workloads.
- **GitOps and rollout:** Argo CD synchronization was exercised against a local Kubernetes cluster. For production, pin tool/image versions, establish GitOps bootstrap ownership, separate environments, and introduce rollout gates with meaningful application SLOs.

The implementation intentionally prioritizes the supplied platform-layer requirements and clear operational reasoning within the exercise's 2–3-hour budget rather than adding Go application behavior or deploying external cloud services.
