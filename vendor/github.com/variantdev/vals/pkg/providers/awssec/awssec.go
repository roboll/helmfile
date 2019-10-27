package awssec

import (
	"errors"
	"fmt"
	"github.com/variantdev/vals/pkg/api"
	"gopkg.in/yaml.v3"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

type provider struct {
	// Keeping track of secretsmanager services since we need a service per region
	client *secretsmanager.SecretsManager

	// AWS SecretsManager global configuration
	Region, VersionStage, VersionId string

	Format string
}

func New(cfg api.StaticConfig) *provider {
	p := &provider{}
	p.Region = cfg.String("region")
	p.VersionStage = cfg.String("version_stage")
	p.VersionId = cfg.String("version_id")
	return p
}

// Get gets an AWS SSM Parameter Store value
func (p *provider) GetString(key string) (string, error) {
	cli := p.getClient()

	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(key),
	}

	if p.VersionStage != "" {
		in = in.SetVersionStage(p.VersionStage)
	}

	if p.VersionId != "" {
		in = in.SetVersionId(p.VersionId)
	}

	out, err := cli.GetSecretValue(in)
	if err != nil {
		return "", fmt.Errorf("get parameter: %v", err)
	}

	var v string
	if out.SecretString != nil {
		v = *out.SecretString
	} else if out.SecretBinary != nil {
		v = string(out.SecretBinary)
	} else {
		return "", errors.New("awssecrets: get secret value: no SecretString nor SecretBinary is set")
	}

	p.debugf("awssecrets: successfully retrieved key=%s", key)

	return v, nil
}

func (p *provider) GetStringMap(key string) (map[string]interface{}, error) {

	yamlStr, err := p.GetString(key)
	if err == nil {
		m := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(yamlStr), &m); err != nil {
			return nil, fmt.Errorf("error while parsing secret for key %q as yaml: %v", key, err)
		}
		return m, nil
	}

	meta := map[string]interface{}{}

	metaKey := strings.TrimRight(key, "/") + "/meta"

	str, err := p.GetString(metaKey)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal([]byte(str), &meta); err != nil {
		return nil, err
	}

	metaKeysField := "github.com/variantdev/vals"
	f, ok := meta[metaKeysField]
	if !ok {
		return nil, fmt.Errorf("%q not found", metaKeysField)
	}

	var suffixes []string
	switch f := f.(type) {
	case []string:
		for _, v := range f {
			suffixes = append(suffixes, v)
		}
	case []interface{}:
		for _, v := range f {
			suffixes = append(suffixes, fmt.Sprintf("%v", v))
		}
	default:
		return nil, fmt.Errorf("%q was not a kind of array: value=%v, type=%T", suffixes, suffixes, suffixes)
	}
	if !ok {
		return nil, fmt.Errorf("%q was not a string array", metaKeysField)
	}

	res := map[string]interface{}{}
	for _, suf := range suffixes {
		sufKey := strings.TrimLeft(suf, "/")
		full := strings.TrimRight(key, "/") + "/" + sufKey
		str, err := p.GetString(full)
		if err != nil {
			return nil, err
		}
		res[sufKey] = str
	}

	p.debugf("SSM: successfully retrieved key=%s", key)

	return res, nil
}

func (p *provider) debugf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func (p *provider) getClient() *secretsmanager.SecretsManager {
	if p.client != nil {
		return p.client
	}

	var cfg *aws.Config
	if p.Region != "" {
		cfg = aws.NewConfig().WithRegion(p.Region)
	} else {
		cfg = aws.NewConfig()
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		panic(err)
	}
	p.client = secretsmanager.New(sess)
	return p.client
}
