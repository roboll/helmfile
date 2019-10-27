package sops

import (
	"fmt"
	"os"

	"github.com/variantdev/vals/pkg/api"
	"gopkg.in/yaml.v3"

	"go.mozilla.org/sops/decrypt"
)

type provider struct {
	// KeyType is either "filepath"(default) or "base64".
	KeyType string
	// Format is --input-type of sops
	Format string
}

func New(cfg api.StaticConfig) *provider {
	p := &provider{}
	p.Format = cfg.String("format")
	p.KeyType = cfg.String("key_type")
	if p.KeyType == "" {
		p.KeyType = "filepath"
	}
	return p
}

// Get gets an AWS SSM Parameter Store value
func (p *provider) GetString(key string) (string, error) {
	cleartext, err := p.decrypt(key, p.format("binary"))
	if err != nil {
		return "", err
	}
	return string(cleartext), nil
}

func (p *provider) GetStringMap(key string) (map[string]interface{}, error) {
	cleartext, err := p.decrypt(key, p.format("yaml"))
	if err != nil {
		return nil, err
	}

	res := map[string]interface{}{}

	if err := yaml.Unmarshal(cleartext, &res); err != nil {
		return nil, err
	}

	p.debugf("sops: successfully retrieved key=%s", key)

	return res, nil
}

func (p *provider) format(defaultFormat string) string {
	if p.Format != "" {
		return p.Format
	}
	return defaultFormat
}

func (p *provider) decrypt(keyOrData, format string) ([]byte, error) {
	if p.KeyType == "base64" {
		return decrypt.Data([]byte(keyOrData), format)
	} else if p.KeyType == "filepath" {
		return decrypt.File(keyOrData, format)
	} else {
		return nil, fmt.Errorf("unsupported key type %q. It must be one \"base64\" or \"filepath\"", p.KeyType)
	}
}

func (p *provider) debugf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}
