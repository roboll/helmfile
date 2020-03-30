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

### 