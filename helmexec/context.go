package helmexec

import (
	"os"
	"path/filepath"
)

type HelmContext struct {
	Tillerless      bool
	TillerNamespace string
	WorkerIndex     int
}

func (context *HelmContext) GetTillerlessArgs(helmBinary string) []string {
	if context.Tillerless {
		if context.TillerNamespace != "" {
			return []string{"tiller", "run", context.TillerNamespace, "--", helmBinary}
		} else {
			return []string{"tiller", "run", "--", helmBinary}
		}
	} else {
		return []string{}
	}
}

func (context *HelmContext) getTillerlessEnv() map[string]string {
	if context.Tillerless {
		result := map[string]string{
			"HELM_TILLER_SILENT": "true",
			// Changing the TILLER port doesn't really work: https://github.com/helm/helm/issues/3159
			// So this is not used for the moment.
			// "HELM_TILLER_PORT":   strconv.Itoa(44134 + context.WorkerIndex),
		}
		if config := os.Getenv("KUBECONFIG"); config != "" {
			absConfig, err := filepath.Abs(config)
			if err == nil {
				result["KUBECONFIG"] = absConfig
			}
		}
		return result
	} else {
		return map[string]string{}
	}
}
