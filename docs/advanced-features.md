## Advanced Features

- [Import Configuration Parameters into Helmfile](#import-configuration-parameters-into-helmfile)

### Import Configuration Parameters into Helmfile

Helmfile integrates [vals]() to import configuration parameters from following backends:

- AWS SSM Parameter Store
- AWS SecretsManager
- Vault
- SOPS

See [Vals "Suported Backends"](https://github.com/variantdev/vals#suported-backends) for the full list of available backends.

This feature was implemented in https://github.com/roboll/helmfile/pull/906.
If you're curious how it's designed and how it works, please consult the pull request.

### Deploy Kustomizations with Helmfile

You can deploy [kustomize](https://github.com/kubernetes-sigs/kustomize) "kustomization"s with Helmfile.

Most of Kustomize operations that is usually done with `kustomize edit` can be done declaratively via Helm values.yaml files.

Under the hood, Helmfile transforms the kustomization into a local chart in a temporary directory so that it can be `helm upgrade --install`ed.

The transformation is done by generating (1)a temporary kustomization from various options and (2)temporary chart from the temporary kustomization.

An example pseudo code for the transformation logic can be written as:

```console
$ TMPCHART=/tmp/sometmpdir
$ mkdir -p ${TMPCHART}/templates
$ somehow_generate_chart_yaml ${TMPCHART}/Chart.yaml

$ TMPKUSTOMIZATION=/tmp/sometmpdir2
$ somehow_generate_temp_kustomization_yaml ${TMPKUSTOMIZATION}/kustomization.yaml
$ kustomize build ${TMPKUSTOMIZATION}/kustomization.yaml > ${TMPCHART}/templates/all.yaml 
```

Let's say you have a `helmfile.yaml` that looks like the below:

```yaml
releases:
- name: myapp
  chart: mykustomization
  values:
  - values.yaml
```

Helmfile firstly generates a temporary `kustomization.yaml` that looks like:

```yaml
bases:
- $(ABS_PATH_TO_HELMFILE_YAML}/mykustomization
```

Followed by the below steps:

- Running `kustomize edit set image $IMAGE` for every `$IMAGE` generated from your values.yaml 
- Running `kustomize edit set nameprefix $NAMEPREFIX` with the nameprefix specified in your values.yaml
- Running `kustomize edit set namesuffix $NAMESUFFIX` with the namesuffix specified in your values.yaml
- Running `kustomize edit set namespace $NS` with the namespace specified in your values.yaml

A `values.yaml` file for kustomization would look like the below:

```yaml
images:
# kustomize edit set image mysql=eu.gcr.io/my-project/mysql@canary
- name: mysql
  newName: eu.gcr.io/my-project/mysql
  newTag: canary
# kustomize edit set image myapp=my-registry/my-app@sha256:24a0c4b4a4c0eb97a1aabb8e29f18e917d05abfe1b7a7c07857230879ce7d3d3
- name: myapp
  digest: sha256:24a0c4b4a4c0eb97a1aabb8e29f18e917d05abfe1b7a7c07857230879ce7d3d3
  newName: my-registry/my-app

# kustomize edit set nameprefix foo-
namePrefix: foo-

# kustomize edit set namesuffix -bar
nameSuffix: -bar

# kustomize edit set namespace myapp
namespace: myapp
```

At this point, Helmfile can generate a complete kustomization from the base kustomization you specified in `releases[].chart` of your helmfile.yaml and `values.yaml`,
which can be included in the temporary chart.

After all, Helmfile just installs the temporary chart like standard charts, which allows you to manage everything with Helmfile regardless of each app is declared using a Helm chart or a kustomization.

Please also see [test/advanced/helmfile.yaml](https://github.com/roboll/helmfile/tree/master/test/advanced/helmfile.yaml) for an example of kustomization support and more.

## Adhoc Customization of Helm charts

You can add/update any Kubernetes resource field rendered from a Helm chart by specifying `releases[].strategicMergePatches`:

```
repositories:
- name: incubator
  url: https://kubernetes-charts-incubator.storage.googleapis.com

releases:
- name: raw1
  chart: incubator/raw
  values:
  - resources:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: raw1
        namespace: default
      data:
        foo: FOO
  strategicMergePatches:
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: raw1
        namespace: default
      data:
        bar: BAR
```

Running `helmfile template` on the above example results in a ConfigMap called `raw` whose `data` is:

```yaml
foo: FOO
bar: BAR
```

Please note that the second `data` field `bar` is coming from the strategic-merge patch defined in the above helmfile.yaml.

There's also `releases[].jsonPatches` that works similarly to `strategicMergePatches` but has additional capability to remove fields.

Please also see [test/advanced/helmfile.yaml](https://github.com/roboll/helmfile/tree/master/test/advanced/helmfile.yaml) for an example of patching support and more.

You can also use templates instead of fix values in the resources:

```
repositories:
- name: incubator
  url: https://kubernetes-charts-incubator.storage.googleapis.com

releases:
- name: raw1
  chart: incubator/raw
  values:
  - templates:
      - |
        apiVersion: v1
        kind: Secret
        metadata:
          name: common-secret
        stringData:
          mykey: {{ .Values.mysecret }}

```
Please also see [test/integration/values.yaml](https://github.com/roboll/helmfile/blob/master/test/integration/values.yaml) for an example of supporting templates.
