# helmfile [![CircleCI](https://circleci.com/gh/roboll/helmfile.svg?style=svg)](https://circleci.com/gh/roboll/helmfile)

Deploy Kubernetes Helm Charts

[![Docker Repository on Quay](https://quay.io/repository/roboll/helmfile/status "Docker Repository on Quay")](https://quay.io/repository/roboll/helmfile)

## about

Helmfile is a declarative spec for deploying helm charts. It lets you...

* Keep a directory of chart value files and maintain changes in version control.
* Apply CI/CD to configuration changes.
* Periodically sync to avoid skew in environments.

To avoid upgrades for each iteration of `helm`, the `helmfile` executable delegates to `helm` - as a result, `helm` must be installed.

The default helmfile is `charts.yaml`:

```
repositories:
  - name: roboll
    url: http://roboll.io/charts

charts:
  - name: vault                          # helm deployment name
    namespace: vault                     # target namespace
    chart: roboll/vault-secret-manager   # chart reference
    values: [ vault.yaml ]               # value files (--values)
    set:                                 # values (--set)
      - name: address
        value: https://vault.example.com
    env:                                 # values (--set) but value will be pulled from environment variables. Will throw an error if the environment variable is not set.
      - name: db.password
        value: DB_PASSWORD               # $DB_PASSOWRD needs to be set in the calling environment ex: export DB_PASSWORD='password1'

```

## install

`go get github.com/roboll/helmfile` or [releases](https://github.com/roboll/helmfile/releases) or [container](https://quay.io/roboll/helmfile)


## usage

```
NAME:
   helmfile -

USAGE:
   main [global options] command [command options] [arguments...]

VERSION:
   0.1.0

COMMANDS:
     repos   sync repositories from state file (helm repo add && helm repo update)
     charts  sync charts from state file (helm repo upgrade --install)
     sync    sync all resources from state file (repos && charts)
     delete  delete charts from state file (helm delete)

GLOBAL OPTIONS:
   --file FILE, -f FILE  load config from FILE (default: "charts.yaml")
   --quiet, -q           silence output
   --kube-context value  Set kubectl context. Uses current context by default
   --help, -h            show help
   --version, -v         print the version
```
