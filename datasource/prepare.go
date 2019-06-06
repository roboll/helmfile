package datasource

import (
	"os"
	"strings"

	"github.com/roboll/helmfile/pkg/helmexec"
	"go.uber.org/zap/zapcore"

	"go.uber.org/zap"
)

var (
	logger *zap.SugaredLogger
)

// PrepareAll prepares all datasources regardless if they're getting used. This only "prepares"
// them, this shouldn't actually instantiate anything that requires network calls or heafty
// cpu/memory usage
func PrepareAll(fileOrDir string) {
	setupLogging()

	SSMPrepare(fileOrDir)
}

func setupLogging() {
	// This isn't ideal because 'log-level' string might change in the app declaration, but the
	// injection point of PrepareAll doesn't provide context to urfav.cli (which has the global flags)
	logLevel := getArgVal("log-level", zapcore.InfoLevel.String())

	logger = helmexec.NewLogger(os.Stdout, logLevel)
}

func getArgVal(key, def string) (val string) {
	val = def
	key = strings.ToLower(key)

	ind := 0
	for i, arg := range os.Args {
		if strings.Contains(strings.ToLower(arg), key) {
			ind = i + 1
			break
		}
	}

	if ind > 0 && ind < len(os.Args) {
		val = os.Args[ind]
	}

	return
}
