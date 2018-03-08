# helmfile [![CircleCI](https://circleci.com/gh/roboll/helmfile.svg?style=svg)](https://circleci.com/gh/roboll/helmfile)

Deploy Kubernetes Helm Charts

[![Docker Repository on Quay](https://quay.io/repository/roboll/helmfile/status "Docker Repository on Quay")](https://quay.io/repository/roboll/helmfile)

## about

Helmfile is a declarative spec for deploying helm charts. It lets you...

* Keep a directory of chart value files and maintain changes in version control.
* Apply CI/CD to configuration changes.
* Periodically sync to avoid skew in environments.

To avoid upgrades for each iteration of `helm`, the `helmfile` executable delegates to `helm` - as a result, `helm` must be installed.

The default helmfile is `helmfile.yaml`:

```
repositories:
  - name: roboll
    url: http://roboll.io/charts
    cert-file: optional_client_cert
    key-file: optional_client_key

context: kube-context					 # kube-context (--kube-context)

releases:
  # Published chart example
  - name: vault                            # name of this release
    namespace: vault                       # target namespace
    chart: roboll/vault-secret-manager     # the chart being installed to create this release, referenced by `repository/chart` syntax
    values: [ vault.yaml ]                 # value files (--values)
    set:                                   # values (--set)
      - name: address
        value: https://vault.example.com
      - name: db.password
        value: {{ env "DB_PASSWORD" }}                   # value taken from environment variable. Will throw an error if the environment variable is not set. $DB_PASSWORD needs to be set in the calling environment ex: export DB_PASSWORD='password1'
      - name: proxy.domain
        value: "{{ env \"PLATFORM_ID\" }}.my-domain.com" # Interpolate environment variable with a fixed string

  # Local chart example
  - name: grafana                            # name of this release
    namespace: another                       # target namespace
    chart: ../my-charts/grafana              # the chart being installed to create this release, referenced by relative path to local chart
    values:
    - "../../my-values/grafana/values.yaml"             # Values file (relative path to manifest)
    - "./values/{{ env \"PLATFORM_ENV\" }}/config.yaml" # Values file taken from path with environment variable. $PLATFORM_ENV must be set in the calling environment.

```

## install

`go get github.com/roboll/helmfile` or [releases](https://github.com/roboll/helmfile/releases) or [container](https://quay.io/roboll/helmfile)


## usage

```
NAME:
   helmfile -

USAGE:
   helmfile [global options] command [command options] [arguments...]

COMMANDS:
     repos   sync repositories from state file (helm repo add && helm repo update)
     charts  sync charts from state file (helm repo upgrade --install)
     diff    diff charts from state file against env (helm diff)
     sync    sync all resources from state file (repos && charts)
     delete  delete charts from state file (helm delete)

GLOBAL OPTIONS:
   --file FILE, -f FILE  load config from FILE (default: "helmfile.yaml")
   --quiet, -q           silence output
   --kube-context value  Set kubectl context. Uses current context by default
   --help, -h            show help
   --version, -v         print the version
```

### diff

The `helmfile diff` sub-command executes the [helm-diff](https://github.com/databus23/helm-diff) plugin across all of
the charts/releases defined in the manifest.

Under the covers Helmfile is simply using the `helm diff` plugin, so that needs to be installed prior.  For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/databus23/helm-diff`. For more details
please look at their [documentation](https://github.com/databus23/helm-diff#helm-diff-plugin).


## Paths Overview
Using manifest files in conjunction with command line argument can be a bit confusing.  

A few rules to clear up this ambiguity: 

- Absolute paths are always resolved as absolute paths
- Relative paths referenced *in* the helmfile manifest itself are relative to that manifest
- Relative paths referenced on the command line are relative to the current working directory the user is in

For additional context, take a look at [paths examples](PATHS.md)
