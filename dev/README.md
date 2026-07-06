# kubeloop dev environment (Loop A, cycle 1)

Prereqs: docker, kind, kubectl, helm.

    make dev-up      # cluster + prometheus + wasteful workloads
    make prom        # prometheus at http://localhost:9090
    make dev-down    # tear it all down

Notes:
- Let metrics accumulate 30+ minutes before the first KRR/kubeloop run;
  for real percentile quality, hours are better (production target: 7 days).
- KRR gate test (Loop A slice 2): install KRR, run it against this cluster
  with the port-forwarded Prometheus, and save the output -- that's the
  gate evidence for POSITIONING.md.
- Total requests ~= 5 CPU / 5.5Gi across 2 workers. If pods sit Pending on a
  small machine, lower replicas or requests in dev/workloads/.
- What each workload tests: padded-idle -> clean waste signal; memory-hog ->
  partial memory waste; worker-bursty -> P99 vs average + confidence;
  nightly-report CronJob -> default exclusion logic; cache StatefulSet ->
  non-Deployment kinds.
