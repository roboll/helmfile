# Helmfile [![CircleCI](https://circleci.com/gh/roboll/helmfile.svg?style=svg)](https://circleci.com/gh/roboll/helmfile)

Deploy Kubernetes Helm Charts

[![Docker Repository on Quay](https://quay.io/repository/roboll/helmfile/status "Docker Repository on Quay")](https://quay.io/repository/roboll/helmfile)
[![Slack Community #helmfile](https://slack.sweetops.com/badge.svg)](https://slack.sweetops.com)

## Status

Even though Helmfile is used in production environments [across multiple organizations](USERS.md), it is still in its early stage of development, hence versioned 0.x.

Helmfile complies to Semantic Versioning 2.0.0 in which v0.x means that there could be backward-incompatible changes for every release.

Note that we will try our best to document any backward incompatibility. And in reality, helmfile had no breaking change for a year or so.

## About

Helmfile is a declarative spec for deploying helm charts. It lets you...

* Keep a directory of chart value files and maintain changes in version control.
* Apply CI/CD to configuration changes.
* Periodically sync to avoid skew in environments.

To avoid upgrades for each iteration of `helm`, the `helmfile` executable delegates to `helm` - as a result, `helm` must be installed.

## Highlights

**Declarative**: Write, version-control, apply the desired state file for visibility and reproducibility.

**Modules**: Modularize common patterns of your infrastructure, distribute it via Git, S3, etc. to be reused across the entire company (See [#648](https://github.com/roboll/helmfile/pull/648))

**Versatility**: Manage your cluster consisting of charts, [kustomizations](https://github.com/kubernetes-sigs/kustomize), and directories of Kubernetes resources, turning everything to Helm releases (See [#673](https://github.com/roboll/helmfile/pull/673))

**Patch**: JSON/Strategic-Merge Patch Kubernetes resources before `helm-install`ing, without forking upstream charts (See [#673](https://github.com/roboll/helmfile/pull/673))

## Configuration

**CAUTION**: This documentation is for the development version of Helmfile. If you are looking for the documentation for any of releases, please switch to the corresponding release tag like [v0.92.1](https://github.com/roboll/helmfile/tree/v0.92.1).

The default name for a helmfile is `helmfile.yaml`:

```yaml
# Chart repositories used from within this state file
#
# Use `helm-s3` and `helm-git` and whatever Helm Downloader plugins
# to use repositories other than the official repository or one backend by chartmuseum.
repositories:
# To use official "stable" charts a.k.a https://github.com/helm/charts/tree/master/stable
- name: stable
  url: https://kubernetes-charts.storage.googleapis.com
# To use official "incubator" charts a.k.a https://github.com/helm/charts/tree/master/incubator
- name: incubator
  url: https://kubernetes-charts-incubator.storage.googleapis.com
# helm-git powered repository: You can treat any Git repository as a charts repository
- name: polaris
  url: git+https://github.com/reactiveops/polaris@deploy/helm?ref=master
# Advanced configuration: You can setup basic or tls auth
- name: roboll
  url: http://roboll.io/charts
  certFile: optional_client_cert
  keyFile: optional_client_key
  username: optional_username
  password: optional_password
# Advanced configuration: You can use a ca bundle to use an https repo
# with a self-signed certificate
- name: insecure
   url: https://charts.my-insecure-domain.com
   caFile: optional_ca_crt

# context: kube-context # this directive is deprecated, please consider using helmDefaults.kubeContext

# Default values to set for args along with dedicated keys that can be set by contributors, cli args take precedence over these.
# In other words, unset values results in no flags passed to helm.
# See the helm usage (helm SUBCOMMAND -h) for more info on default values when those flags aren't provided.
helmDefaults:
  tillerNamespace: tiller-namespace  #dedicated default key for tiller-namespace
  tillerless: false                  #dedicated default key for tillerless
  kubeContext: kube-context          #dedicated default key for kube-context (--kube-context)
  cleanupOnFail: false               #dedicated default key for helm flag --cleanup-on-fail
  # additional and global args passed to helm (default "")
  args:
    - "--set k=v"
  # verify the chart before upgrading (only works with packaged charts not directories) (default false)
  verify: true
  # wait for k8s resources via --wait. (default false)
  wait: true
  # time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks, and waits on pod/pvc/svc/deployment readiness) (default 300)
  timeout: 600
  # performs pods restart for the resource if applicable (default false)
  recreatePods: true
  # forces resource update through delete/recreate if needed (default false)
  force: false
  # enable TLS for request to Tiller (default false)
  tls: true
  # path to TLS CA certificate file (default "$HELM_HOME/ca.pem")
  tlsCACert: "path/to/ca.pem"
  # path to TLS certificate file (default "$HELM_HOME/cert.pem")
  tlsCert: "path/to/cert.pem"
  # path to TLS key file (default "$HELM_HOME/key.pem")
  tlsKey: "path/to/key.pem"
  # limit the maximum number of revisions saved per release. Use 0 for no limit. (default 10)
  historyMax: 10
  # when using helm 3.2+, automatically create release namespaces if they do not exist (default true)
  createNamespace: true


# The desired states of Helm releases.
#
# Helmfile runs various helm commands to converge the current state in the live cluster to the desired state defined here.
releases:
  # Published chart example
  - name: vault                            # name of this release
    namespace: vault                       # target namespace
    createNamespace: true                  # helm 3.2+ automatically create release namespace (default true)
    labels:                                # Arbitrary key value pairs for filtering releases
      foo: bar
    chart: roboll/vault-secret-manager     # the chart being installed to create this release, referenced by `repository/chart` syntax
    version: ~1.24.1                       # the semver of the chart. range constraint is supported
    condition: vault.enabled               # The values lookup key for filtering releases. Corresponds to the boolean value of `vault.enabled`, where `vault` is an arbitrary value
    missingFileHandler: Warn # set to either "Error" or "Warn". "Error" instructs helmfile to fail when unable to find a values or secrets file. When "Warn", it prints the file and continues.
    # Values files used for rendering the chart
    values:
      # Value files passed via --values
      - vault.yaml
      # Inline values, passed via a temporary values file and --values, so that it doesn't suffer from type issues like --set
      - address: https://vault.example.com
      # Go template available in inline values and values files.
      - image:
          # The end result is more or less YAML. So do `quote` to prevent number-like strings from accidentally parsed into numbers!
          # See https://github.com/roboll/helmfile/issues/608
          tag: {{ requiredEnv "IMAGE_TAG" | quote }}
          # Otherwise:
          #   tag: "{{ requiredEnv "IMAGE_TAG" }}"
          #   tag: !!string {{ requiredEnv "IMAGE_TAG" }}
        db:
          username: {{ requiredEnv "DB_USERNAME" }}
          # value taken from environment variable. Quotes are necessary. Will throw an error if the environment variable is not set. $DB_PASSWORD needs to be set in the calling environment ex: export DB_PASSWORD='password1'
          password: {{ requiredEnv "DB_PASSWORD" }}
        proxy:
          # Interpolate environment variable with a fixed string
          domain: {{ requiredEnv "PLATFORM_ID" }}.my-domain.com
          scheme: {{ env "SCHEME" | default "https" }}
    # Use `values` whenever possible!
    # `set` translates to helm's `--set key=val`, that is known to suffer from type issues like https://github.com/roboll/helmfile/issues/608
    set:
    # single value loaded from a local file, translates to --set-file foo.config=path/to/file
    - name: foo.config
      file: path/to/file
    # set a single array value in an array, translates to --set bar[0]={1,2}
    - name: bar[0]
      values:
      - 1
      - 2
    # set a templated value
    - name: namespace
      value: {{ .Namespace }}
    # will attempt to decrypt it using helm-secrets plugin
    secrets:
      - vault_secret.yaml
    # Override helmDefaults options for verify, wait, timeout, recreatePods and force.
    verify: true
    wait: true
    timeout: 60
    recreatePods: true
    force: false
    # set `false` to uninstall this release on sync.  (default true)
    installed: true
    # restores previous state in case of failed release (default false)
    atomic: true
    # when true, cleans up any new resources created during a failed release (default false)
    cleanupOnFail: false
    # name of the tiller namespace (default "")
    tillerNamespace: vault
    # if true, will use the helm-tiller plugin (default false)
    tillerless: false
    # enable TLS for request to Tiller (default false)
    tls: true
    # path to TLS CA certificate file (default "$HELM_HOME/ca.pem")
    tlsCACert: "path/to/ca.pem"
    # path to TLS certificate file (default "$HELM_HOME/cert.pem")
    tlsCert: "path/to/cert.pem"
    # path to TLS key file (default "$HELM_HOME/key.pem")
    tlsKey: "path/to/key.pem"
    # --kube-context to be passed to helm commands
    # CAUTION: this doesn't work as expected for `tilerless: true`.
    # See https://github.com/roboll/helmfile/issues/642
    # (default "", which means the standard kubeconfig, either ~/kubeconfig or the file pointed by $KUBECONFIG environment variable)
    kubeContext: kube-context
    # limit the maximum number of revisions saved per release. Use 0 for no limit (default 10)
    historyMax: 10

  # Local chart example
  - name: grafana                            # name of this release
    namespace: another                       # target namespace
    chart: ../my-charts/grafana              # the chart being installed to create this release, referenced by relative path to local helmfile
    values:
    - "../../my-values/grafana/values.yaml"             # Values file (relative path to manifest)
    - ./values/{{ requiredEnv "PLATFORM_ENV" }}/config.yaml # Values file taken from path with environment variable. $PLATFORM_ENV must be set in the calling environment.
    wait: true

#
# Advanced Configuration: Nested States
#
helmfiles:
- # Path to the helmfile state file being processed BEFORE releases in this state file
  path: path/to/subhelmfile.yaml
  # Label selector used for filtering releases in the nested state.
  # For example, `name=prometheus` in this context is equivalent to processing the nested state like
  #   helmfile -f path/to/subhelmfile.yaml -l name=prometheus sync
  selectors:
  - name=prometheus
  # Override state values
  values:
  # Values files merged into the nested state's values
  - additional.values.yaml
  # One important aspect of using values here is that they first need to be defined in the values section
  # of the origin helmfile, so in this example key1 needs to be in the values or environments.NAME.values of path/to/subhelmfile.yaml
  # Inline state values merged into the nested state's values
  - key1: val1
- # All the nested state files under `helmfiles:` is processed in the order of definition.
  # So it can be used for preparation for your main `releases`. An example would be creating CRDs required by `releases` in the parent state file.
  path: path/to/mycrd.helmfile.yaml
- # Terraform-module-like URL for importing a remote directory and use a file in it as a nested-state file
  # The nested-state file is locally checked-out along with the remote directory containing it.
  # Therefore all the local paths in the file are resolved relative to the file
  path: git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0

#
# Advanced Configuration: Environments
#

# The list of environments managed by helmfile.
#
# The default is `environments: {"default": {}}` which implies:
#
# - `{{ .Environment.Name }}` evaluates to "default"
# - `{{ .Values }}` being empty
environments:
  # The "default" environment is available and used when `helmfile` is run without `--environment NAME`.
  default:
    # Everything from the values.yaml is available via `{{ .Values.KEY }}`.
    # Suppose `{"foo": {"bar": 1}}` contained in the values.yaml below,
    # `{{ .Values.foo.bar }}` is evaluated to `1`.
    values:
    - environments/default/values.yaml
    # Each entry in values can be either a file path or inline values.
    # The below is an example of inline values, which is merged to the `.Values`
    - myChartVer: 1.0.0-dev
  # Any environment other than `default` is used only when `helmfile` is run with `--environment NAME`.
  # That is, the "production" env below is used when and only when it is run like `helmfile --environment production sync`.
  production:
    values:
    - environment/production/values.yaml
    - myChartVer: 1.0.0
    # disable vault release processing
    - vault:
        enabled: false
    ## `secrets.yaml` is decrypted by `helm-secrets` and available via `{{ .Environment.Values.KEY }}`
    secrets:
    - environment/production/secrets.yaml
    # Instructs helmfile to fail when unable to find a environment values file listed under `environments.NAME.values`.
    #
    # Possible values are  "Error", "Warn", "Info", "Debug". The default is "Error".
    #
    # Use "Warn", "Info", or "Debug" if you want helmfile to not fail when a values file is missing, while just leaving
    # a message about the missing file at the log-level.
    missingFileHandler: Error

#
# Advanced Configuration: Layering
#
# Helmfile merges all the "base" state files and this state file before processing.
#
# Assuming this state file is named `helmfile.yaml`, all the files are merged in the order of:
#   environments.yaml <- defaults.yaml <- templates.yaml <- helmfile.yaml
bases:
- environments.yaml
- defaults.yaml
- templates.yaml

#
# Advanced Configuration: API Capabilities
#
# 'helmfile template' renders releases locally without querying an actual cluster,
# and in this case `.Capabilities.APIVersions` cannot be populated.
# When a chart queries for a specific CRD, this can lead to unexpected results.
#
# Configure a fixed list of api versions to pass to 'helm template' via the --api-versions flag:
apiVersions:
- example/v1

```

## Templating

Helmfile uses [Go templates](https://godoc.org/text/template) for templating your helmfile.yaml. While go ships several built-in functions, we have added all of the functions in the [Sprig library](https://godoc.org/github.com/Masterminds/sprig).

We also added one special template function: `requiredEnv`.
The `requiredEnv` function allows you to declare a particular environment variable as required for template rendering.
If the environment variable is unset or empty, the template rendering will fail with an error message.

## Using environment variables

Environment variables can be used in most places for templating the helmfile. Currently this is supported for `name`, `namespace`, `value` (in set), `values` and `url` (in repositories).

Examples:

```yaml
repositories:
- name: your-private-git-repo-hosted-charts
  url: https://{{ requiredEnv "GITHUB_TOKEN"}}@raw.githubusercontent.com/kmzfs/helm-repo-in-github/master/
```

```yaml
releases:
  - name: {{ requiredEnv "NAME" }}-vault
    namespace: {{ requiredEnv "NAME" }}
    chart: roboll/vault-secret-manager
    values:
      - db:
          username: {{ requiredEnv "DB_USERNAME" }}
          password: {{ requiredEnv "DB_PASSWORD" }}
    set:
      - name: proxy.domain
        value: {{ requiredEnv "PLATFORM_ID" }}.my-domain.com
      - name: proxy.scheme
        value: {{ env "SCHEME" | default "https" }}
```

## installation

- download one of [releases](https://github.com/roboll/helmfile/releases) or
- run as a [container](https://quay.io/roboll/helmfile) or
- install from [AUR](https://aur.archlinux.org/packages/kubernetes-helmfile-bin/) for Archlinux or
- Windows (using [scoop](https://scoop.sh/)): `scoop install helmfile`
- macOS (using [homebrew](https://brew.sh/)): `brew install helmfile`

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
helmfile apply
```

Congratulations! You now have your first Prometheus deployment running inside your cluster.

Iterate on the `helmfile.yaml` by referencing:

- [Configuration](#configuration)
- [CLI reference](#cli-reference).
- [Helmfile Best Practices Guide](https://github.com/roboll/helmfile/blob/master/docs/writing-helmfile.md)

## cli reference

```
NAME:
   helmfile

USAGE:
   helmfile [global options] command [command options] [arguments...]

VERSION:
   v0.92.1

COMMANDS:
     deps      update charts based on the contents of requirements.yaml
     repos     sync repositories from state file (helm repo add && helm repo update)
     charts    DEPRECATED: sync releases from state file (helm upgrade --install)
     diff      diff releases from state file against env (helm diff)
     template  template releases from state file against env (helm template)
     lint      lint charts from state file (helm lint)
     sync      sync all resources from state file (repos, releases and chart deps)
     apply     apply all resources from state file only when there are changes
     status    retrieve status of releases in state file
     delete    DEPRECATED: delete releases from state file (helm delete)
     destroy   deletes and then purges releases
     test      test releases from state file (helm test)
     build     output compiled helmfile state(s) as YAML
     list      list releases defined in state file
     help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --helm-binary value, -b value           path to helm binary (default: "helm")
   --file helmfile.yaml, -f helmfile.yaml  load config from file or directory. defaults to helmfile.yaml or `helmfile.d`(means `helmfile.d/*.yaml`) in this preference
   --environment default, -e default       specify the environment name. defaults to default
   --state-values-set value                set state values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)
   --state-values-file value               specify state values in a YAML file
   --quiet, -q                             Silence output. Equivalent to log-level warn
   --kube-context value                    Set kubectl context. Uses current context by default
   --no-color                              Output without color
   --log-level value                       Set log level, default info
   --namespace value, -n value             Set namespace. Uses the namespace set in the context by default, and is available in templates as {{ .Namespace }}
   --selector value, -l value              Only run using the releases that match labels. Labels can take the form of foo=bar or foo!=bar.
                                           A release must match all labels in a group in order to be used. Multiple groups can be specified at once.
                                           --selector tier=frontend,tier!=proxy --selector tier=backend. Will match all frontend, non-proxy releases AND all backend releases.
                                           The name of a release can be used as a label. --selector name=myrelease
   --allow-no-matching-release             Do not exit with an error code if the provided selector has no matching releases.
   --interactive, -i                       Request confirmation before attempting to modify clusters
   --help, -h                              show help
   --version, -v                           print the version
```

### sync

The `helmfile sync` sub-command sync your cluster state as described in your `helmfile`. The default helmfile is `helmfile.yaml`, but any YAML file can be passed by specifying a `--file path/to/your/yaml/file` flag.

Under the covers, Helmfile executes `helm upgrade --install` for each `release` declared in the manifest, by optionally decrypting [secrets](#secrets) to be consumed as helm chart values. It also updates specified chart repositories and updates the
dependencies of any referenced local charts.

For Helm 2.9+ you can use a username and password to authenticate to a remote repository.

### deps

The `helmfile deps` sub-command locks your helmfile state and local charts dependencies.

It basically runs `helm dependency update` on your helmfile state file and all the referenced local charts, so that you get a "lock" file per each helmfile state or local chart.

All the other `helmfile` sub-commands like `sync` use chart versions recorded in the lock files, so that e.g. untested chart versions won't suddenly get deployed to the production environment.

For example, the lock file for a helmfile state file named `helmfile.1.yaml` will be `helmfile.1.lock`. The lock file for a local chart would be `requirements.lock`, which is the same as `helm`.

It is recommended to version-control all the lock files, so that they can be used in the production deployment pipeline for extra reproducibility.

To bring in chart updates systematically, it would also be a good idea to run `helmfile deps` regularly, test it, and then update the lock files in the version-control system.

### diff

The `helmfile diff` sub-command executes the [helm-diff](https://github.com/databus23/helm-diff) plugin across all of
the charts/releases defined in the manifest.

To supply the diff functionality Helmfile needs the [helm-diff](https://github.com/databus23/helm-diff) plugin v2.9.0+1 or greater installed. For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/databus23/helm-diff`. For more details
please look at their [documentation](https://github.com/databus23/helm-diff#helm-diff-plugin).

### apply

The `helmfile apply` sub-command begins by executing `diff`. If `diff` finds that there is any changes, `sync` is executed. Adding `--interactive` instructs Helmfile to request your confirmation before `sync`.

An expected use-case of `apply` is to schedule it to run periodically, so that you can auto-fix skews between the desired and the current state of your apps running on Kubernetes clusters.

### destroy

The `helmfile destroy` sub-command deletes and purges all the releases defined in the manifests.

`helmfile --interactive destroy` instructs Helmfile to request your confirmation before actually deleting releases.

`destroy` basically runs `helm delete --purge` on all the targeted releases. If you don't want purging, use `helmfile delete` instead.

### delete (DEPRECATED)

The `helmfile delete` sub-command deletes all the releases defined in the manifests.

`helmfile --interactive delete` instructs Helmfile to request your confirmation before actually deleting releases.

Note that `delete` doesn't purge releases. So `helmfile delete && helmfile sync` results in sync failed due to that releases names are not deleted but preserved for future references. If you really want to remove releases for reuse, add `--purge` flag to run it like `helmfile delete --purge`.


### secrets

The `secrets` parameter in a `helmfile.yaml` causes the [helm-secrets](https://github.com/futuresimple/helm-secrets) plugin to be executed to decrypt the file.

To supply the secret functionality Helmfile needs the `helm secrets` plugin installed. For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/futuresimple/helm-secrets
`.

### test

The `helmfile test` sub-command runs a `helm test` against specified releases in the manifest, default to all

Use `--cleanup` to delete pods upon completion.

### lint

The `helmfile lint` sub-command runs a `helm lint` across all of the charts/releases defined in the manifest. Non local charts will be fetched into a temporary folder which will be deleted once the task is completed.

## Paths Overview
Using manifest files in conjunction with command line argument can be a bit confusing.

A few rules to clear up this ambiguity:

- Absolute paths are always resolved as absolute paths
- Relative paths referenced *in* the Helmfile manifest itself are relative to that manifest
- Relative paths referenced on the command line are relative to the current working directory the user is in

For additional context, take a look at [paths examples](PATHS.md)

## Labels Overview
A selector can be used to only target a subset of releases when running Helmfile. This is useful for large helmfiles with releases that are logically grouped together.

Labels are simple key value pairs that are an optional field of the release spec. When selecting by label, the search can be inverted. `tier!=backend` would match all releases that do NOT have the `tier: backend` label. `tier=fronted` would only match releases with the `tier: frontend` label.

Multiple labels can be specified using `,` as a separator. A release must match all selectors in order to be selected for the final helm command.

The `selector` parameter can be specified multiple times. Each parameter is resolved independently so a release that matches any parameter will be used.

`--selector tier=frontend --selector tier=backend` will select all the charts

In addition to user supplied labels, the name, the namespace, and the chart are available to be used as selectors.  The chart will just be the chart name excluding the repository (Example `stable/filebeat` would be selected using `--selector chart=filebeat`).

## Templates

You can use go's text/template expressions in `helmfile.yaml` and `values.yaml.gotmpl` (templated helm values files). `values.yaml` references will be used verbatim. In other words:

- for value files ending with `.gotmpl`, template expressions will be rendered
- for plain value files (ending in `.yaml`), content will be used as-is

In addition to built-in ones, the following custom template functions are available:

- `readFile` reads the specified local file and generate a golang string
- `fromYaml` reads a golang string and generates a map
- `setValueAtPath PATH NEW_VALUE` traverses a golang map, replaces the value at the PATH with NEW_VALUE
- `toYaml` marshals a map into a string

### Values Files Templates

You can reference a template of values file in your `helmfile.yaml` like below:

```yaml
releases
- name: myapp
  chart: mychart
  values:
  - values.yaml.gotmpl
```

Every values file whose file extension is `.gotmpl` is considered as a template file.

Suppose `values.yaml.gotmpl` was something like:

```yaml
{{ readFile "values.yaml" | fromYaml | setValueAtPath "foo.bar" "FOO_BAR" | toYaml }}
```

And `values.yaml` was:

```yaml
foo:
  bar: ""
```

The resulting, temporary values.yaml that is generated from `values.yaml.tpl` would become:

```yaml
foo:
  # Notice `setValueAtPath "foo.bar" "FOO_BAR"` in the template above
  bar: FOO_BAR
```

## Refactoring `helmfile.yaml` with values files templates

One of expected use-cases of values files templates is to keep `helmfile.yaml` small and concise.

See the example `helmfile.yaml` below:

```yaml
releases:
  - name: {{ requiredEnv "NAME" }}-vault
    namespace: {{ requiredEnv "NAME" }}
    chart: roboll/vault-secret-manager
    values:
      - db:
          username: {{ requiredEnv "DB_USERNAME" }}
          password: {{ requiredEnv "DB_PASSWORD" }}
    set:
      - name: proxy.domain
        value: {{ requiredEnv "PLATFORM_ID" }}.my-domain.com
      - name: proxy.scheme
        value: {{ env "SCHEME" | default "https" }}
```

The `values` and `set` sections of the config file can be separated out into a template:

`helmfile.yaml`:

```yaml
releases:
  - name: {{ requiredEnv "NAME" }}-vault
    namespace: {{ requiredEnv "NAME" }}
    chart: roboll/vault-secret-manager
    values:
    - values.yaml.gotmpl
```

`values.yaml.gotmpl`:

```yaml
db:
  username: {{ requiredEnv "DB_USERNAME" }}
  password: {{ requiredEnv "DB_PASSWORD" }}
proxy:
  domain: {{ requiredEnv "PLATFORM_ID" }}.my-domain.com
  scheme: {{ env "SCHEME" | default "https" }}
```

## Environment

When you want to customize the contents of `helmfile.yaml` or `values.yaml` files per environment, use this feature.

You can define as many environments as you want under `environments` in `helmfile.yaml`.

The environment name defaults to `default`, that is, `helmfile sync` implies the `default` environment.
The selected environment name can be referenced from `helmfile.yaml` and `values.yaml.gotmpl` by `{{ .Environment.Name }}`.

If you want to specify a non-default environment, provide a `--environment NAME` flag to `helmfile` like `helmfile --environment production sync`.

The below example shows how to define a production-only release:

```yaml
environments:
  default:
  production:

releases:
- name: newrelic-agent
  installed: {{ eq .Environment.Name "production" | toYaml }}
  # snip
- name: myapp
  # snip
```

## Environment Values

Environment Values allows you to inject a set of values specific to the selected environment, into values.yaml templates.
Use it to inject common values from the environment to multiple values files, to make your configuration DRY.

Suppose you have three files `helmfile.yaml`, `production.yaml` and `values.yaml.gotmpl`:

`helmfile.yaml`

```yaml
environments:
  production:
    values:
    - production.yaml

releases:
- name: myapp
  values:
  - values.yaml.gotmpl
```

`production.yaml`

```yaml
domain: prod.example.com
releaseName: prod
```

`values.yaml.gotmpl`

```yaml
domain: {{ .Values | getOrNil "my.domain" | default "dev.example.com" }}
```

`helmfile sync` installs `myapp` with the value `domain=dev.example.com`,
whereas `helmfile --environment production sync` installs the app with the value `domain=production.example.com`.

For even more flexibility, you can now use values declared in the `environments:` section in other parts of your helmfiles:

consider:
`default.yaml`

```yaml
domain: dev.example.com
releaseName: dev
```

```yaml
environments:
  default:
    values:
    - default.yaml
  production:
    values:
    - production.yaml #  bare .yaml file, content will be used verbatim
    - other.yaml.gotmpl  #  template directives with potential side-effects like `exec` and `readFile` will be honoured

releases:
- name: myapp-{{ .Values.releaseName }} # release name will be one of `dev` or `prod` depending on selected environment
  values:
  - values.yaml.gotmpl
- name: production-specific-release
  # this release would be installed only if selected environment is `production`
  installed: {{ eq .Values.releaseName "prod" | toYaml }}
  ...
```

### Note

The `{{ .Values.foo }}` syntax is the recommended way of using environment values.

Prior to this [pull request](https://github.com/roboll/helmfile/pull/647), environment values were made available through the `{{ .Environment.Values.foo }}` syntax.
This is still working but is **deprecated** and the new `{{ .Values.foo }}` syntax should be used instead.

You can read more infos about the feature proposal [here](https://github.com/roboll/helmfile/issues/640).

## Environment Secrets

Environment Secrets (not to be confused with Kubernetes Secrets) are encrypted versions of `Environment Values`.
You can list any number of `secrets.yaml` files created using `helm secrets` or `sops`, so that
Helmfile could automatically decrypt and merge the secrets into the environment values.

First you must have the [helm-secrets](https://github.com/futuresimple/helm-secrets) plugin installed along with a
`.sops.yaml` file to configure the method of encryption (this can be in the same directory as your helmfile or
in the sub-directory containing your secrets files).

Then suppose you have a a foo.bar secret defined in `environments/production/secrets.yaml`:
```yaml
foo.bar: "mysupersecretstring"
```

You can then encrypt it with `helm secrets enc environments/production/secrets.yaml`

Then reference that encrypted file in `helmfile.yaml`:
```yaml
environments:
  production:
    secrets:
    - environments/production/secrets.yaml

releases:
- name: myapp
  chart: mychart
  values:
  - values.yaml.gotmpl
```

Then the environment secret `foo.bar` can be referenced by the below template expression in your `values.yaml.gotmpl`:

```yaml
{{ .Values.foo.bar }}
```

## Tillerless

With the [helm-tiller](https://github.com/rimusz/helm-tiller) plugin installed, you can work without tiller installed.

To enable this mode, you need to define `tillerless: true` and set the `tillerNamespace` in the `helmDefaults` section
or in the `releases` entries.

## DAG-aware installation/deletion ordering

`needs` controls the order of the installation/deletion of the release:

```yaml
releases:
- name: somerelease
  needs:
  - [TILLER_NAMESPACE/][NAMESPACE/]anotherelease
```

All the releases listed under `needs` are installed before(or deleted after) the release itself.

For the following example, `helmfile [sync|apply]` installs releases in this order:

1. logging
2. servicemesh
3. myapp1 and myapp2

```yaml
  - name: myapp1
    chart: charts/myapp
    needs:
    - servicemesh
    - logging
  - name: myapp2
    chart: charts/myapp
    needs:
    - servicemesh
    - logging
  - name: servicemesh
    chart: charts/istio
    needs:
    - logging
  - name: logging
    chart: charts/fluentd
```

Note that all the releases in a same group is installed concurrently. That is, myapp1 and myapp2 are installed concurrently.

On `helmfile [delete|destroy]`, deletions happen in the reverse order.

That is, `myapp1` and `myapp2` are deleted first, then `servicemesh`, and finally `logging`.

## Separating helmfile.yaml into multiple independent files

Once your `helmfile.yaml` got to contain too many releases,
split it into multiple yaml files.

Recommended granularity of helmfile.yaml files is "per microservice" or "per team".
And there are two ways to organize your files.

- Single directory
- Glob patterns

### Single directory

`helmfile -f path/to/directory` loads and runs all the yaml files under the specified directory, each file as an independent helmfile.yaml.
The default helmfile directory is `helmfile.d`, that is,
in case helmfile is unable to locate `helmfile.yaml`, it tries to locate `helmfile.d/*.yaml`.

All the yaml files under the specified directory are processed in the alphabetical order. For example, you can use a `<two digit number>-<microservice>.yaml` naming convention to control the sync order.

- `helmfile.d`/
  - `00-database.yaml`
  - `00-backend.yaml`
  - `01-frontend.yaml`

### Glob patterns

In case you want more control over how multiple `helmfile.yaml` files are organized, use `helmfiles:` configuration key in the `helmfile.yaml`:

Suppose you have multiple microservices organized in a Git repository that looks like:

- `myteam/` (sometimes it is equivalent to a k8s ns, that is `kube-system` for `clusterops` team)
  - `apps/`
    - `filebeat/`
      - `helmfile.yaml` (no `charts/` exists, because it depends on the stable/filebeat chart hosted on the official helm charts repository)
      - `README.md` (each app managed by my team has a dedicated README maintained by the owners of the app)
    - `metricbeat/`
      - `helmfile.yaml`
      - `README.md`
    - `elastalert-operator/`
      - `helmfile.yaml`
      - `README.md`
      - `charts/`
        - `elastalert-operator/`
          - `<the content of the local helm chart>`

The benefits of this structure is that you can run `git diff` to locate in which directory=microservice a git commit has changes.
It allows your CI system to run a workflow for the changed microservice only.

A downside of this is that you don't have an obvious way to sync all microservices at once. That is, you have to run:

```bash
for d in apps/*; do helmfile -f $d diff; if [ $? -eq 2 ]; then helmfile -f $d sync; fi; done
```

At this point, you'll start writing a `Makefile` under `myteam/` so that `make sync-all` will do the job.

It does work, but you can rely on the Helmfile feature instead.

Put `myteam/helmfile.yaml` that looks like:

```yaml
helmfiles:
- apps/*/helmfile.yaml
```

So that you can get rid of the `Makefile` and the bash snippet.
Just run `helmfile sync` inside `myteam/`, and you are done.

All the files are sorted alphabetically per group = array item inside `helmfiles:`, so that you have granular control over ordering, too.

#### selectors

When composing helmfiles you can use selectors from the command line as well as explicit selectors inside the parent helmfile to filter the releases to be used.

```yaml
helmfiles:
- apps/*/helmfile.yaml
- path: apps/a-helmfile.yaml
  selectors:          # list of selectors
  - name=prometheus
  - tier=frontend
- path: apps/b-helmfile.yaml # no selector, so all releases are used
selectors: []
- path: apps/c-helmfile.yaml # parent selector to be used or cli selector for the initial helmfile
  selectorsInherited: true
```

* When a selector is specified, only this selector applies and the parents or CLI selectors are ignored.
* When not selector is specified there are 2 modes for the selector inheritance because we would like to change the current inheritance behavior (see [issue #344](https://github.com/roboll/helmfile/issues/344)  ).
  * Legacy mode, sub-helmfiles without selectors inherit selectors from their parent helmfile. The initial helmfiles inherit from the command line selectors.
  * explicit mode, sub-helmfile without selectors do not inherit from their parent or the CLI selector. If you want them to inherit from their parent selector then use `selectorsInherited: true`. To enable this explicit mode you need to set the following environment variable `HELMFILE_EXPERIMENTAL=explicit-selector-inheritance` (see [experimental](#experimental-features)).
* Using `selector: []` will select all releases regardless of the parent selector or cli for the initial helmfile
* using `selectorsInherited: true` make the sub-helmfile selects releases with the parent selector or the cli for the initial helmfile. You cannot specify an explicit selector while using `selectorsInherited: true`

## Importing values from any source

The `exec` template function that is available in `values.yaml.gotmpl` is useful for importing values from any source
that is accessible by running a command:

A usual usage of `exec` would look like this:

```
mysetting: |
{{ exec "./mycmd" (list "arg1" "arg2" "--flag1") | indent 2 }}
```

Or even with a pipeline:

```
mysetting: |
{{ yourinput | exec "./mycmd-consume-stdin" (list "arg1" "arg2") | indent 2 }}
```

The possibility is endless. Try importing values from your golang app, bash script, jsonnet, or anything!

## Hooks

A Helmfile hook is a per-release extension point that is composed of:

- `events`
- `command`
- `args`
- `showlogs`

Helmfile triggers various `events` while it is running.
Once `events` are triggered, associated `hooks` are executed, by running the `command` with `args`. The standard output of the `command` will be displayed if `showlogs` is set and it's value is `true`.

Currently supported `events` are:

- `prepare`
- `presync`
- `postsync`
- `cleanup`

Hooks associated to `prepare` events are triggered after each release in your helmfile is loaded from YAML, before execution.

Hooks associated to `cleanup` events are triggered after each release is processed.

Hooks associated to `presync` events are triggered before each release is applied to the remote cluster. This is the ideal event to execute any commands that may mutate the cluster state as it will not be run for read-only operations like `lint`, `diff` or `template`.

Hooks associated to `postsync` events are triggered after each release is applied to the remote cluster. This is the ideal event to execute any commands that may mutate the cluster state as it will not be run for read-only operations like `lint`, `diff` or `template`.

The following is an example hook that just prints the contextual information provided to hook:

```
releases:
- name: myapp
  chart: mychart
  # *snip*
  hooks:
  - events: ["prepare", "cleanup"]
    showlogs: true
    command: "echo"
    args: ["{{`{{.Environment.Name}}`}}", "{{`{{.Release.Name}}`}}", "{{`{{.HelmfileCommand}}`}}\
"]
```

Let's say you ran `helmfile --environment prod sync`, the above hook results in executing:

```
echo {{Environment.Name}} {{.Release.Name}} {{.HelmfileCommand}}
```

Whereas the template expressions are executed thus the command becomes:

```
echo prod myapp sync
```

Now, replace `echo` with any command you like, and rewrite `args` that actually conforms to the command, so that you can integrate any command that does:

- templating
- linting
- testing

For templating, imagine that you created a hook that generates a helm chart on-the-fly by running an external tool like ksonnet, kustomize, or your own template engine.
It will allow you to write your helm releases with any language you like, while still leveraging goodies provided by helm.

### Helmfile + Kustomize

Do you prefer `kustomize` to write and organize your Kubernetes apps, but still want to leverage helm's useful features
like rollback, history, and so on? This section is for you!

The combination of `hooks` and [helmify-kustomize](https://gist.github.com/mumoshu/f9d0bd98e0eb77f636f79fc2fb130690)
enables you to integrate [kustomize](https://github.com/kubernetes-sigs/kustomize) into Helmfile.

That is, you can use `kustomize` to build a local helm chart from a kustomize overlay.

Let's assume you have a kustomize project named `foo-kustomize` like this:

```
foo-kustomize/
├── base
│   ├── configMap.yaml
│   ├── deployment.yaml
│   ├── kustomization.yaml
│   └── service.yaml
└── overlays
    ├── default
    │   ├── kustomization.yaml
    │   └── map.yaml
    ├── production
    │   ├── deployment.yaml
    │   └── kustomization.yaml
    └── staging
        ├── kustomization.yaml
        └── map.yaml

5 directories, 10 files
```

Write `helmfile.yaml`:

```yaml
- name: kustomize
  chart: ./foo
  hooks:
  - events: ["prepare", "cleanup"]
    command: "../helmify"
    args: ["{{`{{if eq .Event.Name \"prepare\"}}build{{else}}clean{{end}}`}}", "{{`{{.Release.Ch\
art}}`}}", "{{`{{.Environment.Name}}`}}"]
```

Run `helmfile --environment staging sync` and see it results in helmfile running `kustomize build foo-kustomize/overlays/staging > foo/templates/all.yaml`.

Voilà! You can mix helm releases that are backed by remote charts, local charts, and even kustomize overlays.

## Guides

Use the [Helmfile Best Practices Guide](/docs/writing-helmfile.md) to write advanced helmfiles that feature:

- Default values
- Layering

We also have dedicated documentation on the following topics which might interest you:

- [Shared Configurations Across Teams](/docs/shared-configuration-across-teams.md)

Or join our friendly slack community in the [`#helmfile`](https://slack.sweetops.com) channel to ask questions and get help. Check out our [slack archive](https://archive.sweetops.com/helmfile/) for good examples of how others are using it.

## Using env files

Helmfile itself doesn't have an ability to load env files. But you can write some bash script to achieve the goal:

```console
set -a; . .env; set +a; helmfile sync
```

Please see #203 for more context.

## Running helmfile interactively

`helmfile --interactive [apply|destroy]` requests confirmation from you before actually modifying your cluster.

Use it when you're running `helmfile` manually on your local machine or a kind of secure administrative hosts.

For your local use-case, aliasing it like `alias hi='helmfile --interactive'` would be convenient.

## Running Helmfile without an Internet connection

Once you download all required charts into your machine, you can run `helmfile charts` to deploy your apps.
It basically run only `helm upgrade --install` with your already-downloaded charts, hence no Internet connection is required.
See #155 for more information on this topic.

## Experimental features
Some experimental features may be available for testing in perspective of being (or not) included in a future release.
Those features are set using the environment variable `HELMFILE_EXPERIMENTAL`. Here is the current experimental feature :
* `explicit-selector-inheritance` : remove today implicit cli selectors inheritance for composed helmfiles, see [composition selector](#selectors)

If you want to enable all experimental features set the env var to `HELMFILE_EXPERIMENTAL=true`

## Azure ACR integration

Azure offers helm repository [support for Azure Container Registry](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-helm-repos) as a preview feature.

To use this you must first `az login` and then `az acr helm repo add -n <MyRegistry>`. This will extract a token for the given ACR and configure `helm` to use it, e.g. `helm repo update` should work straight away.

To use `helmfile` with ACR, on the other hand, you must either include a username/password in the repository definition for the ACR in your `helmfile.yaml` or use the `--skip-deps` switch, e.g. `helmfile template --skip-deps`.

An ACR repository definition in `helmfile.yaml` looks like this:

```
repositories:
  - name: <MyRegistry>
    url: https://<MyRegistry>.azurecr.io/helm/v1/repo
```

## Examples

For more examples, see the [examples/README.md](https://github.com/roboll/helmfile/blob/master/examples/README.md) or the [`helmfile`](https://github.com/cloudposse/helmfiles/tree/master/releases) distribution by [Cloud Posse](https://github.com/cloudposse/).

# Attribution

We use:

- [semtag](https://github.com/pnikosis/semtag) for automated semver tagging. I greatly appreciate the author(pnikosis)'s effort on creating it and their kindness to share it!
