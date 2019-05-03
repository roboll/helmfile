package datasource

import (
	"os"

	"github.com/roboll/helmfile/helmexec"

	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger *zap.SugaredLogger
)

// PrepareAll prepares all datasources regardless if they're getting used. This only "prepares"
// them, this shouldn't actually instantiate anything that requires network calls or heafty
// cpu/memory usage
func PrepareAll(c *cli.Context) {
	setupLogging(c)

	SSMPrepare(c)
}

func setupLogging(c *cli.Context) {
	logLevel := c.GlobalString("log-level")

	if c.GlobalBool("quiet") {
		logLevel = zapcore.WarnLevel.String()
	}

	var level zapcore.Level
	if err := level.Set(logLevel); err != nil {
		level.Set(zapcore.InfoLevel.String())
	}

	logger = helmexec.NewLogger(os.Stdout, logLevel)
}
