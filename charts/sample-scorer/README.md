# Experimental MCH sample-scorer chart

This experimental chart deploys the sample scorer and its optional MCH resources. It is not integrated into MCH's default add-on installation flow.

## Hub-side resources

- `templates/00-hub-sample-scorer.yaml`: namespace, simple scorer Deployment, Service, and OpenShift Route.
- `templates/10-dynamicscorer.yaml` and `12-dynamicscorer-minimum-headroom.yaml`: registrations for the average and minimum CPU-headroom examples. Each API config supplies its query and scoring settings; the CR supplies the Prometheus host fallback and token Secret reference.
- `templates/11-dynamicscoringconfig.yaml`: the current DSF singleton configuration.
- `templates/20-sample-scorer-placement.yaml`, `21-sample-scorer-policy.yaml`, `22-sample-scorer-placementbinding.yaml`, and `23-sample-scorer-managedclustersetbinding.yaml`: select labeled spokes and grant the sample Policy access to the global ManagedClusterSet.

`DynamicScorer` and `DynamicScoringConfig` use MCH's `global.namespace`, populated from the BackplaneConfig target namespace. The sample scorer Deployment, Service, Route, Policy, Placement, PlacementBinding, and ClusterSetBinding are grouped in `dynamic-scoring-sample`; the spoke Prometheus-reader ServiceAccount and token remain in the DSF agent's `dynamic-scoring` namespace. The Route, `DynamicScorer.configURL`, and scoring endpoint share one host from MCH's `hubconfig.clusterIngressDomain`. MCH obtains that value from the hub's `Ingress/cluster` resource. `tests/values.yaml` provides local fixture values only.

The chart defaults to `quay.io/open-cluster-management/dynamic-scoring-framework-sample-scorer:latest`. Override `sampleScorer.image.repository`, `sampleScorer.image.tag`, and `sampleScorer.image.pullPolicy` to deploy an image built or published elsewhere.

The default Prometheus host is configurable with `sampleScorer.prometheusHost`. The current DSF `DynamicScorer` API sends one host to all managed clusters; supporting different Prometheus URLs per spoke requires a DSF framework enhancement.

`sampleScorer.scheme` selects `http` or `https` for the Route and the config/scoring endpoints for both examples. It defaults to `http` for this test setup because the default OpenShift ingress certificate is not trusted by DSF. Use `https` only when the Route presents a certificate trusted by both the hub controller and spoke agents.

Render the chart locally with:

```sh
helm lint ./charts/sample-scorer -f ./charts/sample-scorer/tests/values.yaml
helm template sample-scorer ./charts/sample-scorer -f ./charts/sample-scorer/tests/values.yaml
```

The rendered output can be inspected as YAML. Do not apply the raw template files directly with `kubectl`.

## Spoke-side resources

- `templates/20-sample-scorer-placement.yaml`: selects OpenShift spokes where the DSF add-on is available, excluding `local-cluster`.
- `templates/21-sample-scorer-policy.yaml`: creates the Prometheus-reader ServiceAccount, its `cluster-monitoring-view` binding, and the token Secret on selected spokes.
- `templates/22-sample-scorer-placementbinding.yaml`: binds that Placement to the Policy.

The selected clusters must have the Governance Policy Framework and `config-policy-controller` addons enabled, and the `dynamic-scoring` namespace must exist there. The DSF add-on supplies its availability label automatically; the sample scorer chart does not need a separate opt-in label.

## Known gaps to resolve

- **Placement is not yet a DSF distribution input.** The Placement above selects spokes for the Policy only. The current `DynamicScoringConfig` still distributes to all managed clusters except those explicitly masked; its `local-cluster` mask does not make its targets match the Placement. DSF Placement support is needed for one shared target selection.
- The sample chart uses `${scoreName}` as its score dimension. Its default Prometheus query aggregates to one cluster-wide series per scoring call; if you customize the query to return multiple series, update the dimension format or the results will collide in `AddOnPlacementScore`.
- MCH does not coordinate readiness between the Policy and scorer configuration. If the token Secret arrives after the agent receives its ConfigMap, the agent currently retries on ConfigMap resync (about 10 minutes). A targeted Secret refresh/retry change is planned separately.
- Deleting a Policy does not necessarily delete objects it enforced. The optional bundle needs an explicit cleanup strategy for the spoke ServiceAccount, ClusterRoleBinding, and token Secret when disabled.
- `cluster-monitoring-view` is OpenShift-specific. This Policy/Placement should only target OpenShift spokes.
