.RECIPEPREFIX := >
CLUSTER := kubeloop-dev

dev-up: ## create kind cluster + prometheus + wasteful workloads
> kind create cluster --config dev/kind-config.yaml
> helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true
> helm repo update
> helm upgrade --install kps prometheus-community/kube-prometheus-stack \
>   -n monitoring --create-namespace \
>   --set grafana.enabled=false --set alertmanager.enabled=false --wait
> kubectl apply -f dev/workloads/
> @echo ""
> @echo "Cluster up. Let metrics accumulate (30+ min) before first scan."
> @echo "Prometheus:  make prom   ->  http://localhost:9090"

workloads: ## (re)apply the sample wasteful workloads
> kubectl apply -f dev/workloads/

prom: ## port-forward prometheus to localhost:9090
> kubectl -n monitoring port-forward svc/kps-kube-prometheus-stack-prometheus 9090:9090

dev-down: ## delete the kind cluster
> kind delete cluster --name $(CLUSTER)

help:
> @grep -E '^[a-zA-Z_-]+:.*##' Makefile | sed 's/:.*##/  -/'
