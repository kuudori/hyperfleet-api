# Konflux build failure alerts

The four PipelineRuns in this directory build the API image and chart:

| PipelineRun | Trigger |
| --- | --- |
| `hyperfleet-api-on-push` | Push to `main` |
| `hyperfleet-api-on-tag` | Version tag |
| `hyperfleet-api-chart-on-push` | Chart push |
| `hyperfleet-api-chart-on-tag` | Chart tag |

Each inline `pipelineSpec` has a `slack-webhook-notification` task in `finally`.
It runs only when `$(tasks.status)` is `Failed`. The Slack message links the
repository, commit, and failed PipelineRun with readable text. A successful
PipelineRun skips this task. A failure before a PipelineRun starts, including a
Pipelines as Code admission failure, cannot trigger it.

The task reads key `hyperfleet-slack-webhook-url` from Secret
`hyperfleet-slack-webhook-notification-secret` in `hyperfleet-tenant` on
`kflux-prd-rh02`. The webhook value must never be committed or printed in logs.
The Secret must be available to the build task Pod. HyperFleet manages this
Secret in its own tenant namespace. The webhook determines the Slack channel.
The existing release notification uses a separate Secret in
`rhtap-releng-tenant`, managed by RelEng.

## Check a missing alert

Open the failed PipelineRun in the [Konflux UI](https://konflux-ui.apps.kflux-prd-rh02.0fk9.p1.openshiftapps.com/)
and inspect its `slack-webhook-notification` final task. Check whether it was
scheduled, whether the Secret and key were mounted, and whether the Slack POST
succeeded. If no PipelineRun exists, inspect the Pipelines as Code check on the
GitHub commit. A successful run should show the final task skipped and produce
no build failure alert.

## Rotate the webhook

1. Obtain a replacement incoming webhook for the agreed team channel through
   the HyperFleet team. Confirm whether the URL is shared with release
   notifications.
2. Update `hyperfleet-tenant/hyperfleet-slack-webhook-notification-secret`, key
   `hyperfleet-slack-webhook-url`, through the team's Secret management process.
   Update its configuration source if one is used. If the URL is shared,
   coordinate the release namespace Secret update with RelEng.
3. With approval for a controlled Konflux run and Slack post, trigger a failed
   PipelineRun and confirm the alert fields, link, and delivery time. Check a
   successful run emits no build failure alert. If sharing the URL, confirm a
   release notification still arrives.
4. Revoke the old webhook only after every consumer has been verified on the
   replacement URL.

See the [HyperFleet notification runbook](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/release/operations/notifications.md)
for build and release notification ownership and troubleshooting.

## Verify pinned task bundles

Every `quay.io/konflux-ci/tekton-catalog/*` reference in these PipelineRuns is
pinned by digest. If a digest is not a Tekton bundle, for example a SARIF scan
report that briefly held the tag upstream, every push and tag pipeline fails at
bundle resolution. The bump diff looks the same as a healthy one, so it needs a
check.

`make verify-tekton-bundles` fetches the manifest of each pinned digest and
applies the same compliance check as the Tekton bundles resolver. It uses the
digest only, needs no credentials, and downloads no layers. Install the tool
with `go install github.com/openshift-hyperfleet/hyperfleet-hooks/cmd/hyperfleet-hooks@v0.3.0`,
or point `HYPERFLEET_HOOKS=/path/to/hyperfleet-hooks` at an existing binary.

The required Prow job `ci/prow/verify-tekton-bundles` runs this target on pull
requests that change `.tekton/` or the `Makefile`, including MintMaker
"Update Konflux references" PRs.

If it fails, re-pin the reported task to a digest that passes, for example the
one its tag points at after upstream fixes it. Check a candidate with
`skopeo inspect --raw docker://<ref-with-digest-only>`.
