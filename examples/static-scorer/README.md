# Static scorer QE example

This directory contains example resources for using the static scorer endpoints
in QE tests. These resources are deliberately not part of the Helm chart.

## Register the scorer

Update the namespace and `configURL` in `dynamicscorer.yaml` for the target
environment, then apply it in the namespace where DSF watches `DynamicScorer`
resources:

```sh
oc apply -f examples/static-scorer/dynamicscorer.yaml
```

The example opts into `clusterName` in scoring requests and uses
`AddOnPlacementScore` as its destination. It assumes the sample scorer chart is
already installed and its static endpoint is reachable at the configured URL.

## Provide spoke scores

Create a ConfigMap named `static-scorer-scores` in the scorer Deployment's
namespace (default: `dynamic-scoring-sample-scorer`):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: static-scorer-scores
  namespace: dynamic-scoring-sample-scorer
data:
  scores.json: |
    {
      "clusters": {
        "spoke-a": 90,
        "spoke-b": 40
      }
    }
```

The scorer rereads the mounted file on each request. ConfigMap updates take
effect after Kubernetes projects them into the pod; QE should allow for that
delay and the agent scoring interval. Configured scores must be whole JSON
numbers from `-100` to `100`, inclusive, matching the AddOnPlacementScore API.

An invalid score only affects requests for that spoke; other valid entries in
the same ConfigMap remain usable. For the spoke with an invalid value, the
scorer returns the default score `0`. Invalid values include `null`, strings,
booleans, fractional numbers, and integers outside `-100` to `100`. For
example, with `{"clusters":{"spoke-a":90,"spoke-b":null}}`, `spoke-a` receives
`90` and `spoke-b` receives the default `0`.

The scorer also returns the default `0` if the ConfigMap or `scores.json` is
missing, the JSON cannot be parsed, the `clusters` object is missing, or the
requesting spoke has no entry. The score is returned as a successful scoring
result, so the agent can publish it to that spoke's `AddOnPlacementScore`.
The Deployment sets `STATIC_SCORER_DEFAULT_VALUE=0` to make the sample default
visible; it is not a chart value intended for general configuration.

Remove the QE registration when it is no longer needed:

```sh
oc delete -f examples/static-scorer/dynamicscorer.yaml
```
