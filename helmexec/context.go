package helmexec

type HelmContext struct {
	Tillerless      bool
	TillerNamespace string
}

func (context *HelmContext) GetPrefixArgs(helmBinary string) []string {
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
