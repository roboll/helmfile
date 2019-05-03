package datasource

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

var (
	// Adding caching for SSM parameters since templates are rendered twice and would do 2x network calls
	ssmParams = map[string]string{}

	// Keeping track of SSM services since we need a SSM service per region
	ssmSvcs = map[string]*ssm.SSM{}
)

func SSMGet(region, key string) (val string, err error) {
	if cachedVal, ok := ssmParams[key]; ok && strings.TrimSpace(cachedVal) != "" {
		val = cachedVal
		return
	}

	actualRegion, err := ssmConfigure(region)
	if err != nil {
		return
	}

	in := ssm.GetParameterInput{
		Name:           aws.String(key),
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

	ssmParams[key] = *out.Parameter.Value
	val = ssmParams[key]
	return
}

func ssmConfigure(region string) (actualRegion string, err error) {
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
		// TODO: Figure out proper logging for this (debug)
		fmt.Println("svcSSM already defined for actualRegion:", actualRegion)
		return
	}

	cfg := aws.NewConfig().WithRegion(actualRegion)
	sess := session.New(cfg)
	ssmSvcs[actualRegion] = ssm.New(sess)

	return
}
