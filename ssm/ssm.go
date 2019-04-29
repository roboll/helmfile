package ssm

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/roboll/helmfile/helmexec"
	"go.uber.org/zap"
)

var (
	svcSSM  *ssm.SSM
	ssmPath string
	logger  *zap.SugaredLogger
)

func Run() {
	logger = helmexec.NewLogger(os.Stdout, "debug")
	ssmPath, exists := os.LookupEnv("SSM_PATH")
	if !exists {
		return
	}

	if strings.TrimSpace(ssmPath) == "" {
		ssmPath = "/"
	}

	logger.Debugf("Attempting to populate environment with SSM values at path: %s", ssmPath)
	if err := configureAWS(); err != nil {
		logger.Error("Failed to configure AWS")
		return
	}

	fmt.Println("SSM path:", ssmPath)
	getSet(ssmPath, "")
}

func configureAWS() (err error) {
	awsRegion := os.Getenv("AWS_REGION")
	awsDefaultRegion := os.Getenv("AWS_DEFAULT_REGION")

	if strings.TrimSpace(awsRegion) == "" && strings.TrimSpace(awsDefaultRegion) == "" {
		logger.Debug("ERROR: $AWS_REGION && $AWS_DEFAULT_REGION are empty (need 1 exported). Unable to set SSM parameters")
		err = errors.New("Bad region env vars")
		return
	}

	region := awsDefaultRegion
	if strings.TrimSpace(awsRegion) != "" {
		region = awsRegion
	}
	cfg := aws.NewConfig().WithRegion(region)

	sess := session.New(cfg)
	svcSSM = ssm.New(sess)

	return
}

func getSet(ssmPath, nextToken string) {
	in := &ssm.GetParametersByPathInput{
		Path:           &ssmPath,
		WithDecryption: aws.Bool(true),
	}

	if nextToken != "" {
		in.SetNextToken(nextToken)
	}

	out, err := svcSSM.GetParametersByPath(in)
	if err != nil {
		logger.Error("Failed getting parameter by path:", err)
		return
	}

	for _, parameter := range out.Parameters {
		setParameter(ssmPath, parameter)
	}

	if out.NextToken != nil {
		getSet(ssmPath, *out.NextToken)
	}

}

func setParameter(ssmPath string, parameter *ssm.Parameter) {
	if parameter == nil {
		logger.Error("SSM parameter is nil")
		return
	}

	name := ""
	if parameter.Name != nil {
		name = *parameter.Name
	}

	value := ""
	if parameter.Value != nil {
		value = *parameter.Value
	}

	length := len(ssmPath)
	if len(ssmPath) == 1 {
		length = 0
	}

	key := strings.Replace(strings.Trim(name[length:], "/"), "/", "_", -1)
	value = strings.Replace(value, "\n", "\\n", -1)

	os.Setenv(key, value)
	logger.Debugf("Setenv: key=%s", key)
}
