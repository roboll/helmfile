# Towards Helmfile 1.0

I'd like to make 3 breaking changes to Helmfile and mark it as 1.0, so that we can better communicate with the current and future Helmfile users about our stance on maintaining Helmfile.

Note that every breaking change should have an easy alternative way to achieve the same goal achieved using the removed functionality.

## The changes in 1.0

1. Forbid the use of `environments` and `releases` within a single helmfile.yaml part
  - Helmfile currently relies on a hack called "double rendering" which no one understands correctly (I suppose) to solve the chicken-and-egg problem of rendering the helmfile template(which requires helmfile to parse `environments` as yaml first) and parsing the rendered helmfile as yaml(which requires helmfile to render the template first).
  - By forcing (or print a big warning) the user to separate helmfile parts for `environments` and `releases`, it's very unlikely Helmfile needs double-rendering at all.
  - After this change, every helmfile.yaml written this way:

    ```
    environments:
      default:
        values:
        - foo: bar
    releases:
    - name: myapp
      chart: charts/myapp
      values:
      - {{ .Values | toYaml | nindent 4 }}
    ```
    must be rewritten like:
    ```
    environments:
      default:
        values:
        - foo: bar
    ---
    releases:
    - name: myapp
      chart: charts/myapp
      values:
      - {{ .Values | toYaml | nindent 4 }}
    ```
    It might not be a deal breaker as you already had to separate helmfile parts when you need to generate `environments` dynamically:
    ```
    environments:
      default:
        values:
        - foo: bar
    ---
    environments:
      default:
        values:
        - {{ .Values | toYaml | nindent 6}}}
        - bar: baz
    ---
    releases:
    - name: myapp
      chart: charts/myapp
      values:
      - {{ .Values | toYaml | nindent 4 }}
    ```
2. Force `.gotmpl` (or `.tpl`) file extension for `helmfile.yaml` in case you want helmfile to render it as a go template before yaml parsing.
  - As the primary maintainer of the project, I'm tired of explaining why Helmfile renders go template expressions embedded in a yaml comment. [The latest example of it](https://github.com/helmfile/helmfile/issues/127).
  - When we first introduced helmfile the ability to render helmfile.yaml as a go template, I did propose it to require `.gotmpl` file extension to enable the feature. But none of active helmfile users and contributors at that time agreed with it (although I didn't have a very strong opinion on the matter either), so we enabled it without the extension. I consider it as a tech debt now.

3. Remove the `--args` flag from the `helmfile` command
  - It has been provided as-is, and never intended to work reliably because it's fundamentally flawed because we have no way to specify which args to be passed to which `helm` commands being called by which `helmfile` command.
  - However, I periodically see a new user finds it's not working and reporting it as a bug([the most recent example](https://github.com/roboll/helmfile/issues/2034#issuecomment-1147059088)). It's not a fault of the user, but out fault that left this broken feature.
  - Every use-case previsouly covered (by any chance) with `--args` should be covered in new `helmfile.yaml` fields or flags.

## After 1.0

We won't add any backward-incompatible changes while in 1.x, as long as it's inevitable to fix unseen important bug(s).

We also quit saying [Helmfile is in its early days](https://github.com/helmfile/helmfile#status) in your README as... it's just untrue today.
