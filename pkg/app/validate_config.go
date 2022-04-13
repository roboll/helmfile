package app

import "errors"

func ValidateConfig(conf ApplyConfigProvider) error {
	if conf.NoColor() && conf.Color() {
		return errors.New("--color and --no-color cannot be specified at the same time")
	}

	return nil
}
