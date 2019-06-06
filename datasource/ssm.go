package datasource

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"gopkg.in/go-playground/validator.v9"
	"gopkg.in/yaml.v2"
)

const (
	ssmKeySeparator = ":"
)

var (
	// Adding caching for SSM parameters since templates are rendered twice and would do 2x network calls
	ssmParams = map[string]string{}

	// Keeping track of SSM services since we need a SSM service per region
	ssmSvcs = map[string]*ssm.SSM{}

	// The file to parse
	ssmFile string

	// The parsed contents of the ssmFile
	ssmHelmfile SSMHelmfile
)

// SSMHelmfile is a helmfile struct with only SSM defined
type SSMHelmfile struct {
	SSMSpecs []SSMSpec `yaml:"ssm"`
}

// SSMSpec defines the AWS SSM Paramter store global configuration
type SSMSpec struct {
	Region string `yaml:"region" validate:"required"`
	Prefix string `yaml:"prefix" validate:"required"`
	Name   string `yaml:"name" validate:"required"`
}

// ExpandEnv runs os.ExpandEnv on all values incase any env vars are added here
func (s *SSMSpec) ExpandEnv() {
	s.Name = os.ExpandEnv(s.Name)
	s.Prefix = os.ExpandEnv(s.Prefix)
	s.Region = os.ExpandEnv(s.Region)
}

// SSMPrepare prepares usage for SSM regardless if ssm is used or not
func SSMPrepare(fileOrDir string) {
	// TODO: Not sure how this works with multiple files/dirs
	ssmFile = fileOrDir
}

// SSMGet gets an AWS SSM Parameter Store value
func SSMGet(key string) (val string, err error) {
	key = os.ExpandEnv(key)

	// Check for cached value
	if cachedVal, ok := ssmParams[key]; ok && strings.TrimSpace(cachedVal) != "" {
		val = cachedVal
		return
	}

	// Check if SSM Specs were parsed from file already
	if ssmHelmfile.SSMSpecs == nil {
		if err = ssmGetSpecs(); err != nil {
			return
		}
	}

	// TODO: Prevent multiple runs of this for cpu optimization? (Not sure if pkg.state.state behaves
	// differently with multiple files)
	if err = expandValidateSSMSpecs(); err != nil {
		return
	}

	// Configure ssmSvcs
	actualRegion, err := ssmConfigure(key)
	if err != nil {
		return
	}

	// Assemble key based on SSMSpec
	assembledKey, err := ssmAssembleKey(key)
	if err != nil {
		return
	}

	in := ssm.GetParameterInput{
		Name:           aws.String(assembledKey),
		WithDecryption: aws.Bool(true),
	}
	out, err := ssmSvcs[actualRegion].GetParameter(&in)
	if err != nil {
		return
	}

	if out.Parameter == nil {
		err = errors.New("datasource.ssm.SSMGet() out.Parameter is nil")
		return
	}

	if out.Parameter.Value == nil {
		err = errors.New("datasource.ssm.SSMGet() out.Parameter.Value is nil")
		return
	}

	// Cache the value
	ssmParams[key] = *out.Parameter.Value
	val = ssmParams[key]

	logger.Debugf("SSM: successfully retrieved key=%s", assembledKey)
	return
}

func ssmConfigure(key string) (actualRegion string, err error) {
	region := ssmGetRegionFromKey(key)

	awsRegion := os.Getenv("AWS_REGION")
	awsDefaultRegion := os.Getenv("AWS_DEFAULT_REGION")

	if strings.TrimSpace(region) == "" && strings.TrimSpace(awsRegion) == "" && strings.TrimSpace(awsDefaultRegion) == "" {
		err = errors.New("ssm[*].region && $AWS_REGION && $AWS_DEFAULT_REGION are empty")
		return
	}

	actualRegion = region
	if strings.TrimSpace(actualRegion) == "" {
		actualRegion = awsRegion
	}

	if strings.TrimSpace(actualRegion) == "" {
		actualRegion = awsDefaultRegion
	}

	if svcSSM, ok := ssmSvcs[actualRegion]; ok && svcSSM != nil {
		logger.Debug("SSM: svcSSM already defined for actualRegion=", actualRegion)
		return
	}

	cfg := aws.NewConfig().WithRegion(actualRegion)
	sess := session.New(cfg)
	ssmSvcs[actualRegion] = ssm.New(sess)

	return
}

func ssmGetRegionFromKey(key string) (region string) {
	keyPieces := strings.Split(key, ssmKeySeparator)
	if len(keyPieces) != 2 {
		return
	}

	ssmSpecName := keyPieces[0]

	for _, ssmSpec := range ssmHelmfile.SSMSpecs {
		if ssmSpec.Name == ssmSpecName {
			region = ssmSpec.Region
			break
		}
	}

	return
}

func expandValidateSSMSpecs() (err error) {
	for ind := range ssmHelmfile.SSMSpecs {
		ssmHelmfile.SSMSpecs[ind].ExpandEnv()
	}

	validate := validator.New()
	for _, ssmSpec := range ssmHelmfile.SSMSpecs {
		if err = validate.Struct(ssmSpec); err != nil {
			return
		}
	}

	return
}

func ssmGetSpecs() (err error) {
	if strings.TrimSpace(ssmFile) == "" {
		logger.Warn("SSM spec missing in helmfile; running with defaults")
		return
	}

	file, err := os.Open(ssmFile)
	if err != nil {
		return
	}
	defer file.Close()

	var tmpSSMHelmfile SSMHelmfile
	if err = yaml.NewDecoder(file).Decode(&tmpSSMHelmfile); err != nil {
		logger.Debug("ERROR: ssmGetSpecs failed to yaml decode file into tmpSSMHelmfile")
		return
	}

	logger.Debug("SSM: successfully got SSMSpec: ", tmpSSMHelmfile.SSMSpecs)
	ssmHelmfile = tmpSSMHelmfile
	return
}

func ssmAssembleKey(key string) (assembledKey string, err error) {
	keyPieces := strings.Split(key, ssmKeySeparator)
	if len(keyPieces) != 2 {
		assembledKey = key
		return
	}

	ssmSpecName := keyPieces[0]
	ssmKey := keyPieces[1]

	ssmPrefix := ""
	for _, ssmSpec := range ssmHelmfile.SSMSpecs {
		if ssmSpec.Name == ssmSpecName {
			ssmPrefix = ssmSpec.Prefix
			break
		}
	}

	if strings.TrimSpace(ssmPrefix) == "" {
		err = fmt.Errorf("SSM.Prefix not found for ssmSpecName=%s", ssmSpecName)
		return
	}

	ssmPrefix = strings.TrimRight(ssmPrefix, "/")
	assembledKey = ssmPrefix + "/" + ssmKey
	return
}
