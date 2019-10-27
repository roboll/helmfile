# values

Helm-like configuration "Values" loader with support for various backends including:

- Vault
- AWS SSM Parameter Store
- AWS Secrets Manager
- [SOPS](https://github.com/mozilla/sops)-encrypted files
- Terraform outputs(Coming soon)
- CredHub(Coming soon)

## Usage

- [CLI](#cli)
- [Helm](#helm)
- [Go](#go)

# CLI

```
vals is a Helm-like configuration "Values" loader with support for various sources and merge strategies

Usage:
  vals [command]

Available Commands:
  eval		Evaluate a JSON/YAML document and replace any template expressions in it and prints the result
  flatten	Loads a vals template and replaces every instances of custom types to plain $ref's
  ksdecode	Decode YAML document(s) by converting Secret resources' "data" to "stringData" for use with "vals eval"

Use "vals [command] --help" for more infomation about a command
```

`vals -t yaml -e <YAML>` takes any valid YAML and evaluates [JSO Reference](https://json-spec.readthedocs.io/reference.html).

`vals` has its own provider which can be reffered with a URI scheme looks `vals+<TYPE>`.

For this example, use the [Vault](https://www.terraform.io/docs/providers/vault/index.html) provider.

Let's start by writing some secret value to `Vault`:

```console
$ vault write mykv/foo mykey=myvalue
```

Now input the template of your YAML and refer to `vals`' Vault provider by using `vals+vault` in the URI scheme:

```console
$ vals eval -e '
foo: ref+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey
bar:
  baz: ref+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey
```

Voila! `vals`, replacing every reference to your secret value in Vault, produces the output looks like:

```yaml
foo: FOO
bar:
  baz: FOO
```

Which is equivalent to that of the following shell script:

```bash
VAULT_TOKEN=yourtoken  VAULT_ADDR=http://127.0.0.1:8200/ cat <<EOF
foo: $(vault read mykv/foo -o json | jq -r .mykey)
  bar:
    baz: $(vault read mykv/foo -o json | jq -r .mykey)
EOF
```

Save the YAML content to `x.vals.yaml` and running `vals eval -f x.vals.yaml` does produce output equivalent to the previous one:

```yaml
foo: FOO
bar:
  baz: FOO
```

### Helm

Use value references as Helm Chart values, so that you can feed the `helm template` output to `vals -f -` for transforming the refs to secrets.

```console
$ helm template mysql-1.3.2.tgz --set mysqlPassword='ref+vault://127.0.0.1:8200/mykv/foo#/mykey' | vals ksdecode -o yaml -f - | tee manifests.yaml
apiVersion: v1
kind: Secret
metadata:
  labels:
    app: release-name-mysql
    chart: mysql-1.3.2
    heritage: Tiller
    release: release-name
  name: release-name-mysql
  namespace: default
stringData:
  mysql-password: refs+vault://127.0.0.1:8200/mykv/foo#/mykey
  mysql-root-password: vZQmqdGw3z
type: Opaque
```

This manifest is safe to be committed into your version-control system(GitOps!) as it doesn't contain actual secrets.

When you finally deploy the manifests, run `vals eval` to replace all the refs to actual secrets:

```console
$ cat manifests.yaml | ~/p/values/bin/vals eval -f - | tee all.yaml
apiVersion: v1
kind: Secret
metadata:
    labels:
        app: release-name-mysql
        chart: mysql-1.3.2
        heritage: Tiller
        release: release-name
    name: release-name-mysql
    namespace: default
stringData:
    mysql-password: myvalue
    mysql-root-password: 0A8V1SER9t
type: Opaque
```

Finally run `kubectl apply` to apply manifests:

```console
$ kubectl apply -f all.yaml
```

This gives you a solid foundation for building a secure CD system as you need to allow access to a secrets store like Vault only from servers or containers that pulls safe manifests and runs deployments.

In other words, you can safely omit access from the CI to the secrets store.

### Go

```go
import "github.com/variantdev/vals"

secretsToCache := 256 // how many secrets to keep in LRU cache
runtime, err := vals.New(secretsToCache)
if err != nil {
  return nil, err
}

valsRendered, err := runtime.Eval(map[string]interface{}{
    "inline": map[string]interface{}{
        "foo": "ref+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey",
        "bar": map[string]interface{}{
            "baz": "ref+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey",
        },
    },
})
```

Now, `vals` contains a `map[string]interface{}` representation of the below:

```console
cat <<EOF
foo: $(vault read mykv/foo -o json | jq -r .mykey)
  bar:
    baz: $(vault read mykv/foo -o json | jq -r .mykey)
EOF
```

## Suported Backends

- Vault
- AWS SSM Parameter Store
- AWS Secrets Manager
- [SOPS](https://github.com/mozilla/sops)

Please see [pkg/providers](https://github.com/variantdev/vals/tree/master/pkg/providers) for the implementations of all the providers. The package names corresponds to the URI schemes.

### Vault

- `ref+vault://PATH/TO/KVBACKEND[?address=VAULT_ADDR:PORT&token_file=PATH/TO/FILE&token_env=VAULT_TOKEN]#/fieldkey`
- `ref+vault://PATH/TO/KVBACKEND[?address=VAULT_ADDR:PORT&token_file=PATH/TO/FILE&token_env=VAULT_TOKEN]#/fieldkey`

`adddress` defaults to the value of the `VAULT_ADDR` envvar.

Examples:

- `ref+vault://mykv/foo#/bar?address=https://vault1.example.com:8200` reads the value for the field `bar` in the kv `foo` on Vault listening on `https://vault1.example.com` with the Vault token read from **the envvar `VAULT_TOKEN`, or the file `~/.vault_token` when the envvar is not set**
- `ref+vault://mykv/foo#/bar?token_env=VAULT_TOKEN_VAULT1&address=https://vault1.example.com:8200` reads the value for the field `bar` in the kv `foo` on Vault listening on `https://vault1.example.com` with the Vault token read from **the envvar `VAULT_TOKEN_VAULT1`**
- `ref+vault://mykv/foo#/bar?token_file=~/.vault_token_vault1&address=https://vault1.example.com:8200` reads the value for the field `bar` in the kv `foo` on Vault listening on `https://vault1.example.com` with the Vault token read from **the file `~/.vault_token_vault1`**

### AWS SSM Parameter Store

- `ref+awsssm://PATH/TO/PARAM[?region=REGION]`
- `ref+awsssm://PREFIX/TO/PARAMS[?region=REGION]#/PATH/TO/PARAM`

In the latter case, `vals` uses `GetParametersByPath(/PREFIX/TO/PARAMS)` caches the result per prefix rather than each single path to reduce number of API calls

Examples:

- `ref+awsssm://myteam/mykey`
- `ref+awsssm://myteam/mydoc#/foo/bar`
- `ref+awsssm://myteam/mykey?region=us-west-2`

### AWS Secrets Manager

- `ref+awssec://PATH/TO/SECRET[?region=REGION&version_stage=STAGE&version_id=ID]`
- `ref+awssec://PATH/TO/SECRET[?region=REGION&version_stage=STAGE&version_id=ID]#/yaml_or_json_key/in/secret`

Examples:

- `ref+awssec://myteam/mykey`
- `ref+awssec://myteam/mydoc#/foo/bar`
- `ref+awssec://myteam/mykey?region=us-west-2`

### SOPS

- The whole content of a SOPS-encrypted file: `ref+sops://base64_data_or_path_to_file?key_type=[filepath|base64]&format=[binary|dotenv|yaml]`
- The value for the specific path in an encrypted YAML/JSON document: `ref+sops://base64_data_or_path_to_file#/json_or_yaml_key/in/the_encrypted_doc`

Examples:

- `ref+sops://path/to/file` reads `path/to/file` as `binary` input
- `ref+sops://<base64>?key_type=base64` reads `<base64>` as the base64-encoded data to be decrypted by sops as `binary`
- `ref+sops://path/to/file#/foo/bar` reads `path/to/file` as a `yaml` file and returns the value at `foo.bar`.
- `ref+sops://path/to/file?format=json#/foo/bar` reads `path/to/file` as a `json` file and returns the value at `foo.bar`.

## Non-Goals

### String-Interpolation / Template Functions

In the early days of this project, the original author has investigated if it was a good idea to introduce string interpolation like feature to vals:

```
foo: xx${{ref "vals+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey" }}
bar:
  baz: yy${{ref "vals+vault://127.0.0.1:8200/mykv/foo?proto=http#/mykey" }}
```

But the idea had abandoned due to that it seemed to drive the momentum to vals being a full-fledged YAML templating engine. What if some users started wanting to use `vals` for transforming values with functions?
That's not the business of vals.

Instead, use vals solely for composing sets of values that are then input to another templating engine or data manipulation language like Jsonnet and CUE.

### Merge

Merging YAMLs is out of the scope of `vals`. There're better alternatives like Jsonnet, Sprig, and CUE for the job.
