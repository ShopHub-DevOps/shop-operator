# Shop Operator

A Kubernetes operator that provisions and manages Shop application instances and their supporting resources on behalf of [ShopHub](https://github.com/ShopHub-DevOps/shophub).

## Custom Resource Definitions

The operator reconciles three CRDs:

| CRD | Purpose |
|---|---|
| `Shop` | Deploys a Shop application with the requested replica count (2 replicas for the `standard` availability tier, 3 for `high`) and database type (PostgreSQL via CNPG, or Redis via REDB). |
| `DiscordChannel` | Creates a Discord channel used for alert notifications for a given shop. |
| `Wallet` | Represents the blockchain account configured to receive customer payments for a given shop. |

## Tech stack

| Layer | Technology |
|---|---|
| Language | Go |
| Framework | kubebuilder / controller-runtime |
| External operators consumed | CNPG (PostgreSQL), REDB (Redis) |
| Container registry | GitHub Container Registry (`ghcr.io/shophub-devops`) |
| Local Kubernetes | kind |

## Deployment

The operator is deployed via its Helm chart in the [helm-charts](https://github.com/ShopHub-DevOps/helm-charts) repository, under `charts/shop-operator`. The chart installs the operator Deployment, the CRD schemas, and the required RBAC.

## Running locally

To be documented once the operator scaffold is in place.

## Related repositories

| Repository | Purpose |
|---|---|
| [shophub](https://github.com/ShopHub-DevOps/shophub) | Platform panel that creates `Shop` custom resources through this operator |
| [shop](https://github.com/ShopHub-DevOps/shop) | The shop application deployed by this operator |
| [helm-charts](https://github.com/ShopHub-DevOps/helm-charts) | Contains the Helm chart for this operator and its CRDs |
| [kube-state](https://github.com/ShopHub-DevOps/kube-state) | Declarative cluster state that installs this operator |

## License

MIT. See [LICENSE](./LICENSE).
