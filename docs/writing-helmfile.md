# The Helmfile Best Practices Guide

This guide covers the Helmfile’s considered patterns for writing advanced helmfiles. It focuses on how helmfile should be structured and executed.

## Missing keys and Default values

helmfile tries its best to inform users for noticing potential mistakes.

One example of how helmfile achieves it is that, `helmfile` fails when you tried to access missing keys in environment values.

That is, the following example let `helmfile` fail when you have no `eventApi.replicas` defined in environment values.

```
{{ .Environment.Values.eventApi.replicas | default 1 }}
```

In case it isn't a mistake and you do want to allow missing keys, use the `getOrNil` template function:

```
{{ .Environment.Values | getOrNil "eventApi.replicas" }}
```

This result in printing `<no value` in your template, that may or may not result in a failure.

If you want a kind of default values that is used when a missing key was referenced, use `default` like:

```
{{ .Environment.Values | getOrNil "eventApi.replicas" | default 1 }}
```

Now, you get `1` when there is no `eventApi.replicas` defined in environment values.

## Release Template / Conventional Directory Structure

Introducing helmfile into a large-scale project that involes dozens of releases often results in a lot of repetitions in `helmfile.yaml` files.

The example below shows repetitions in `namespace`, `chart`, `values`, and `secrets`:

```yaml
releases:
# *snip*
- name: heapster
  namespace: kube-system
  chart: stable/heapster
  version: 0.3.2
  values:
  - "./config/heapster/values.yaml"
  - "./config/heapster/{{ .Environment.Name }}.yaml"
  secrets:
  - "./config/heapster/secrets.yaml"
  - "./config/heapster/{{ .Environment.Name }}-secrets.yaml"

- name: kubernetes-dashboard
  namespace: kube-system
  chart: stable/kubernetes-dashboard
  version: 0.10.0
  values:
  - "./config/kubernetes-dashboard/values.yaml"
  - "./config/kubernetes-dashboard/{{ .Environment.Name }}.yaml"
  values:
  - "./config/kubernetes-dashboard/secrets.yaml"
  - "./config/kubernetes-dashboard/{{ .Environment.Name }}-secrets.yaml"
```

This is where Helmfile's advanced feature called Release Template comes handy.

It allows you to abstract away the repetitions in releases into a template, which is then included and executed by using YAML anchor/alias:

```yaml
templates:
  default: &default
    chart: stable/{{`{{ .Release.Name }}`}}
    namespace: kube-system
    # This prevents helmfile exiting when it encounters a missing file
    # Valid values are "Error", "Warn", "Info", "Debug". The default is "Error"
    # Use "Debug" to make missing files errors invisible at the default log level(--log-level=INFO)
    missingFileHandler: Warn
    values:
    - config/{{`{{ .Release.Name }}`}}/values.yaml
    - config/{{`{{ .Release.Name }}`}}/{{`{{ .Environment.Name }}`}}.yaml
    secrets:
    - config/{{`{{ .Release.Name }}`}}/secrets.yaml
    - config/{{`{{ .Release.Name }}`}}/{{`{{ .Environment.Name }}`}}-secrets.yaml

releases:
- name: heapster
  <<: *default
- name: kubernetes-dashboard
  <<: *default
```

See the [issue 428](https://github.com/roboll/helmfile/issues/428) for more context on how this is supposed to work.

## Layering

You may occasionally end up with many helmfiles that shares common parts like which repositories to use, and whichi release to be bundled by default.

Use Layering to extract the common parts into a dedicated *library helmfile*s, so that each helmfile becomes DRY.

Let's assume that your `helmfile.yaml` looks like:

```
{ readFile "commons.yaml" }}
---
{{ readFile "environments.yaml" }}
---
releases:
- name: myapp
  chart: mychart
```

Whereas `commons.yaml` contained a monitoring agent:

```yaml
releases:
- name: metricbaet
  chart: stable/metricbeat
```

And `environments.yaml` contained well-known environments:

```yaml
environments:
  development:
  production:
```

At run time, template expressions in your `helmfile.yaml` are executed:

```yaml
releases:
- name: metricbaet
  chart: stable/metricbeat
---
environments:
  development:
  production:
---
releases:
- name: myapp
  chart: mychart
```

Resulting YAML documents are merged in the order of occurrence,
so that your `helmfile.yaml` becomes:

```yaml
environments:
  development:
  production:

releases:
- name: metricbaet
  chart: stable/metricbeat
- name: myapp
  chart: mychart
```

Great!

Now, repeat the above steps for each your `helmfile.yaml`, so that all your helmfiles becomes DRY.
