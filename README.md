# helmfile [![CircleCI](https://circleci.com/gh/roboll/helmfile.svg?style=svg)](https://circleci.com/gh/roboll/helmfile)

Deploy Kubernetes Helm Charts

[![Docker Repository on Quay](https://quay.io/repository/roboll/helmfile/status "Docker Repository on Quay")](https://quay.io/repository/roboll/helmfile)

## status

Even though Helmfile is used in production environments [across multiple organizations](USERS.md), it is still in its early stage of development, hence versioned 0.x.

Helmfile complies to Semantic Versioning 2.0.0 in which v0.x means that there could be backward-incompatible changes for every release.

Note that we will try our best to document any backward incompatibility.

## about

Helmfile is a declarative spec for deploying helm charts. It lets you...

* Keep a directory of chart value files and maintain changes in version control.
* Apply CI/CD to configuration changes.
* Periodically sync to avoid skew in environments.

To avoid upgrades for each iteration of `helm`, the `helmfile` executable delegates to `helm` - as a result, `helm` must be installed.

## configuration syntax

**CAUTION**: This documentation is for the development version of Helmfile. If you are looking for the documentation for any of releases, please switch to the corresponding release tag like [v0.31.0](https://github.com/roboll/helmfile/tree/v0.31.0).

The default helmfile is `helmfile.yaml`:

```yaml
repositories:
  - name: roboll
    url: http://roboll.io/charts
    certFile: optional_client_cert
    keyFile: optional_client_key
    username: optional_username
    password: optional_password

# context: kube-context # this directive is deprecated, please consider using helmDefaults.kubeContext

#default values to set for args along with dedicated keys that can be set by contributers, cli args take precedence over these
helmDefaults:
  tillerNamespace: tiller-namespace  #dedicated default key for tiller-namespace
  kubeContext: kube-context          #dedicated default key for kube-context (--kube-context)
  # additional and global args passed to helm
  args:
    - "--set k=v"
  # defaults for verify, wait, force, timeout and recreatePods under releases[]
  verify: true
  wait: true
  timeout: 600
  recreatePods: true
  force: true

releases:
  # Published chart example
  - name: vault                            # name of this release
    namespace: vault                       # target namespace
    labels:                                  # Arbitrary key value pairs for filtering releases
      foo: bar
    chart: roboll/vault-secret-manager     # the chart being installed to create this release, referenced by `repository/chart` syntax
    version: ~1.24.1                       # the semver of the chart. range constraint is supported
    values:
      # value files passed via --values
      - vault.yaml
      # inline values, passed via a temporary values file and --values
      - address: https://vault.example.com
        db:
          username: {{ requiredEnv "DB_USERNAME" }}
          # value taken from environment variable. Quotes are necessary. Will throw an error if the environment variable is not set. $DB_PASSWORD needs to be set in the calling environment ex: export DB_PASSWORD='password1'
          password: {{ requiredEnv "DB_PASSWORD" }}
        proxy:
          # Interpolate environment variable with a fixed string
          domain: {{ requiredEnv "PLATFORM_ID" }}.my-domain.com
          scheme: {{ env "SCHEME" | default "https" }}
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
    # wait for k8s resources via --wait. Defaults to `false`
    wait: true
    # time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks, and waits on pod/pvc/svc/deployment readiness) (default 300)
    timeout: 60
    # performs pods restart for the resource if applicable
    recreatePods: true
    # forces resource update through delete/recreate if needed
    force: true
    # set `false` to uninstall on sync
    installed: true

  # Local chart example
  - name: grafana                            # name of this release
    namespace: another                       # target namespace
    chart: ../my-charts/grafana              # the chart being installed to create this release, referenced by relative path to local chart
    values:
    - "../../my-values/grafana/values.yaml"             # Values file (relative path to manifest)
    - ./values/{{ requiredEnv "PLATFORM_ENV" }}/config.yaml # Values file taken from path with environment variable. $PLATFORM_ENV must be set in the calling environment.
    wait: true

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
respositories:
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

- `go get github.com/roboll/helmfile` or
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

Iterate on the `helmfile.yaml` by referencing the [configuration syntax](#configuration-syntax) and the [cli reference](#cli-reference).

## cli reference

```
NAME:
   helmfile -

USAGE:
   helmfile [global options] command [command options] [arguments...]

COMMANDS:
     repos     sync repositories from state file (helm repo add && helm repo update)
     charts    sync releases from state file (helm upgrade --install)
     diff      diff releases from state file against env (helm diff)
     template  template releases from state file against env (helm template)
     lint      lint charts from state file (helm lint)
     sync      sync all resources from state file (repos, releases and chart deps)
     apply     apply all resources from state file only when there are changes
     status    retrieve status of releases in state file
     delete    delete releases from state file (helm delete)
     test      test releases from state file (helm test)

GLOBAL OPTIONS:
   --helm-binary value, -b value           path to helm binary
   --file helmfile.yaml, -f helmfile.yaml  load config from file or directory. defaults to helmfile.yaml or `helmfile.d`(means `helmfile.d/*.yaml`) in this preference
   --environment default, -e default       specify the environment name. defaults to default
   --quiet, -q                             Silence output. Equivalent to log-level warn
   --kube-context value                    Set kubectl context. Uses current context by default
   --log-level value                       Set log level, default info
   --namespace value, -n value             Set namespace. Uses the namespace set in the context by default, and is available in templates as {{ .Namespace }}
   --selector value, -l value              Only run using the releases that match labels. Labels can take the form of foo=bar or foo!=bar.
                                           A release must match all labels in a group in order to be used. Multiple groups can be specified at once.
                                           --selector tier=frontend,tier!=proxy --selector tier=backend. Will match all frontend, non-proxy releases AND all backend releases.
                                           The name of a release can be used as a label. --selector name=myrelease
   --interactive, -i                       Request confirmation before attempting to modify clusters
   --help, -h                              show help
   --version, -v                           print the version
```

### sync

The `helmfile sync` sub-command sync your cluster state as described in your `helmfile`. The default helmfile is `helmfile.yaml`, but any yaml file can be passed by specifying a `--file path/to/your/yaml/file` flag.

Under the covers, Helmfile executes `helm upgrade --install` for each `release` declared in the manifest, by optionally decrypting [secrets](#secrets) to be consumed as helm chart values. It also updates specified chart repositories and updates the
dependencies of any referenced local charts.

For Helm 2.9+ you can use a username and password to authenticate to a remote repository.

### diff

The `helmfile diff` sub-command executes the [helm-diff](https://github.com/databus23/helm-diff) plugin across all of
the charts/releases defined in the manifest.

To supply the diff functionality Helmfile needs the [helm-diff](https://github.com/databus23/helm-diff) plugin v2.9.0+1 or greater installed. For Helm 2.3+
you should be able to simply execute `helm plugin install https://github.com/databus23/helm-diff`. For more details
please look at their [documentation](https://github.com/databus23/helm-diff#helm-diff-plugin).

### apply

The `helmfile apply` sub-command begins by executing `diff`. If `diff` finds that there is any changes, `sync` is executed. Adding `--interactive` instructs Helmfile to request your confirmation before `sync`.

An expected use-case of `apply` is to schedule it to run periodically, so that you can auto-fix skews between the desired and the current state of your apps running on Kubernetes clusters.

### delete

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
    - values.yaml.tmpl
```

`values.yaml.tmpl`:

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

{{ if eq .Environment.Name "production" }}
- name: newrelic-agent
  # snip
{{ end }}
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
domain: {{ .Environment.Values | getOrNil "my.domain" | default "dev.example.com" }}
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
- name: myapp-{{ .Environment.Values.releaseName }} # release name will be one of `dev` or `prod` depending on selected environment
  values:
  - values.yaml.gotmpl

{{ if eq (.Environment.Values.releaseName "prod" ) }}
# this release would be installed only if selected environment is `production`
- name: production-specific-release
  ...
{{ end }}
```

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
{{ .Environment.Values.foo.bar }}
```

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

Helmfile triggers various `events` while it is running.
Once `events` are triggered, associated `hooks` are executed, by running the `command` with `args`.

Currently supported `events` are:

- `prepare`
- `cleanup`

Hooks associated to `prepare` events are triggered after each release in your helmfile is loaded from YAML, before execution.

Hooks associated to `cleanup` events are triggered after each release is processed.

The following is an example hook that just prints the contextual information provided to hook:

```
releases:
- name: myapp
  chart: mychart
  # *snip*
  hooks:
  - events: ["prepare", "cleanup"]
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

That is, you can use `kustommize` to build a local helm chart from a kustomize overlay.

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

## Using env files

Helmfile itself doesn't have an ability to load env files. But you can write some bash script to achieve the goal:

```console
set -a; . .env; set +a; helmfile sync
```

Please see #203 for more context.

## Running helmfile interactively

`helmfile --interactive [apply|delete]` requests confirmation from you before actually modifying your cluster.

Use it when you're running `helmfile` manually on your local machine or a kind of secure administrative hosts.

For your local use-case, aliasing it like `alias hi='helmfile --interactive'` would be convenient.

## Running Helmfile without an Internet connection

Once you download all required charts into your machine, you can run `helmfile charts` to deploy your apps.
It basically run only `helm upgrade --install` with your already-downloaded charts, hence no Internet connection is required.
See #155 for more information on this topic.


## Examples

For more examples, see the [examples/README.md](https://github.com/roboll/helmfile/blob/master/examples/README.md) or the [`helmfile.d`](https://github.com/cloudposse/helmfiles/tree/master/helmfile.d) distribution of helmfiles by [Cloud Posse](https://github.com/cloudposse/).
