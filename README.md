# helmfile [![CircleCI](https://circleci.com/gh/roboll/helmfile.svg?style=svg)](https://circleci.com/gh/roboll/helmfile)

Deploy Kubernetes Helm Charts

[![Docker Repository on Quay](https://quay.io/repository/roboll/helmfile/status "Docker Repository on Quay")](https://quay.io/repository/roboll/helmfile)

## status

Even though Helmfile is used in production environments across multiple organizations, it is still in its early stage of development, hence versioned 0.x.

Helmfile complies to Semantic Versioning 2.0.0 in which v0.x means that there could be backward-incompatible changes for every release.

Note that we will try our best to document any backward incompatibility.

## about

Helmfile is a declarative spec for deploying helm charts. It lets you...

* Keep a directory of chart value files and maintain changes in version control.
* Apply CI/CD to configuration changes.
* Periodically sync to avoid skew in environments.

To avoid upgrades for each iteration of `helm`, the `helmfile` executable delegates to `helm` - as a result, `helm` must be installed.

## configuration syntax

The default helmfile is `helmfile.yaml`:

```yaml
repositories:
  - name: roboll
    url: http://roboll.io/charts
    certFile: optional_client_cert
    keyFile: optional_client_key

context: kube-context					 # kube-context (--kube-context)

releases:
  # Published chart example
  - name: vault                            # name of this release
    namespace: vault                       # target namespace
    labels:                                  # Arbitrary key value pairs for filtering releases
      foo: bar
    chart: roboll/vault-secret-manager     # the chart being installed to create this release, referenced by `repository/chart` syntax
    version: ~1.24.1                       # the semver of the chart. range constraint is supported
    values: [ vault.yaml ]                 # value files (--values)
    secrets:
      - vault_secret.yaml                  # will attempt to decrypt it using helm-secrets plugin
    set:                                   # values (--set)
      - name: address
        value: https://vault.example.com
      - name: db.password
        value: {{ env "DB_PASSWORD" }}    # value taken from environment variable. Quotes are necessary. Will throw an error if the environment variable is not set. $DB_PASSWORD needs to be set in the calling environment ex: export DB_PASSWORD='password1'
      - name: proxy.domain
        value: {{ env "PLATFORM_ID" }}.my-domain.com # Interpolate environment variable with a fixed string

  # Local chart example
  - name: grafana                            # name of this release
    namespace: another                       # target namespace
    chart: ../my-charts/grafana              # the chart being installed to create this release, referenced by relative path to local chart
    values:
    - "../../my-values/grafana/values.yaml"             # Values file (relative path to manifest)
    - ./values/{{ env "PLATFORM_ENV" }}/config.yaml # Values file taken from path with environment variable. $PLATFORM_ENV must be set in the calling environment.

```

## Using environment variables

Environment variables can be used in most places for templating the helmfile. Currently this is supported for `name`, `namespace`, `value` (in set) and `url` (in repositories).

Examples:

```yaml
respositories:
- name: your-private-git-repo-hosted-charts
  url: https://{{ env "GITHUB_TOKEN"}}@raw.githubusercontent.com/kmzfs/helm-repo-in-github/master/
```

```yaml
releases:
  - name: {{ env "NAME" }}-vault
    namespace: {{ env "NAME" }}
    chart: roboll/vault-secret-manager
    set:
      - name: proxy.domain
        value: {{ env "PLATFORM_ID" }}.my-domain.com
```

## installation

- `go get github.com/roboll/helmfile` or
- download one of [releases](https://github.com/roboll/helmfile/releases) or
- run as a [container](https://quay.io/roboll/helmfile)

## getting started

Let's start with a simple `helmfile` and gradually improve it to fit your use-case!

Suppose the `helmfile.yaml` representing the desired state of your helm releases looks like:

```yaml
releases:
- name: prom-norbac-ubuntu
  namespace: prometheus
  chart: stable/prometheus
  set:
  - name: rbac.create
    value: false
```

Sync your Kubernetes cluster state to the desired one by running:

```console
helmfile sync
```

Congratulations! You now have your first Prometheus deployment running inside your cluster.

Iterate on the `helmfile.yaml` by referencing the [configuration syntax](#configuration-syntax) and the [cli reference](#cli-reference).

## cli reference

```
NAME:
   helmfile -

USAGE:
   helmfile [global options] command [command options] [arguments...]

COMMANDS:
     repos   sync repositories from state file (helm repo add && helm repo update)
     charts  sync charts from state file (helm upgrade --install)
     diff    diff charts from state file against env (helm diff)
     sync    sync all resources from state file (repos, charts and local chart deps)
     delete  delete charts from state file (helm delete)

GLOBAL OPTIONS:
   --file FILE, -f FILE         load config from FILE (default: "helmfile.yaml")
   --quiet, -q                  silence output
   --namespace value, -n value  Set namespace. Uses the namespace set in the context by default
   --selector,l value           Only run using the releases that match labels. Labels can take the form of foo=bar or foo!=bar.
	                              A release must match all labels in a group in order to be used. Multiple groups can be specified at once.
	                              --selector tier=frontend,tier!=proxy --selector tier=backend. Will match all frontend, non-proxy releases AND all backend releases.
	                              The name of a release can be used as a label. --selector name=myrelease
   --kube-context value         Set kubectl context. Uses current context by default
   --help, -h                   show help
   --version, -v                print the version
```

### sync

The `helmfile sync` sub-command sync your cluster state as described in your `helmfile`. The default helmfile is `helmfile.yaml`, but any yaml file can be passed by specifying a `--file path/to/your/yaml/file` flag.

Under the covers, Helmfile executes `helm upgrade --install` for each `release` declared in the manifest, by optionally decrypting [secrets](#secrets) to be consumed as helm chart values. It also updates specified chart repositories and updates the
dependencies of any referenced local charts.

### diff

The `helmfile diff` sub-command executes the [helm-diff](https://github.com/databus23/helm-diff) plugin across all of
the charts/releases defined in the manifest.

To supply the diff functionality Helmfile needs the `helm diff` plugin installed. For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/databus23/helm-diff`. For more details
please look at their [documentation](https://github.com/databus23/helm-diff#helm-diff-plugin).

### secrets

The `secrets` parameter in a `helmfile.yaml` causes the [helm-secrets](https://github.com/futuresimple/helm-secrets) plugin to be executed to decrypt the file.

To supply the secret functionality Helmfile needs the `helm secrets` plugin installed. For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/futuresimple/helm-secrets
`.

## Paths Overview
Using manifest files in conjunction with command line argument can be a bit confusing.

A few rules to clear up this ambiguity:

- Absolute paths are always resolved as absolute paths
- Relative paths referenced *in* the helmfile manifest itself are relative to that manifest
- Relative paths referenced on the command line are relative to the current working directory the user is in

For additional context, take a look at [paths examples](PATHS.md)

## Labels Overview
A selector can be used to only target a subset of releases when running helmfile. This is useful for large helmfiles with releases that are logically grouped together.

Labels are simple key value pairs that are an optional field of the release spec. When selecting by label, the search can be inverted. `tier!=backend` would match all releases that do NOT have the `tier: backend` label. `tier=fronted` would only match releases with the `tier: frontend` label.

Multiple labels can be specified using `,` as a separator. A release must match all selectors in order to be selected for the final helm command.

The `selector` parameter can be specified multiple times. Each parameter is resolved independently so a release that matches any parameter will be used.

`--selector tier=frontend --selector tier=backend` will select all the charts

## Examples

For more examples, see [examples/REAMDME.md](https://github.com/roboll/helmfile/blob/master/examples/README.md).
