package serve

import (
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"github.com/kiry163/mole/cmd/mole/internal"
	dash "github.com/kiry163/mole/pkg/dashboard"
	"github.com/kiry163/mole/pkg/gateway"
	svc "github.com/kiry163/mole/pkg/service"
)

// Options are the serve command startup flags.
type Options struct {
	Debug bool
}

// BindFlags registers the serve flags on fs.
func BindFlags(fs *flag.FlagSet, opts *Options) {
	fs.BoolVarP(&opts.Debug, "debug", "d", false, "Enable debug logging")
}

// runServe starts the gateway in the foreground. Tests replace it to assert the
// flag routing without starting a server. The service program owns environment
// loading, timezone setup, and signal handling so foreground and managed runs
// share one lifecycle.
var runServe = func(opts Options) error {
	spec, err := svc.NewSpec(internal.GetMoleHome(), internal.GetConfigPath())
	if err != nil {
		return err
	}
	program := svc.NewProgram(gateway.Options{
		Debug:      opts.Debug,
		HomePath:   spec.HomePath,
		ConfigPath: spec.ConfigPath,
		Mount: func(env *gateway.RuntimeEnv) error {
			return dash.Mount(env, dash.Options{
				ConfigPath: spec.ConfigPath,
				Debug:      opts.Debug,
			})
		},
	})
	return svc.Run(spec, program)
}

// NewServeCommand returns the foreground gateway command.
func NewServeCommand() *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the gateway and web console in the foreground",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(opts)
		},
	}
	BindFlags(cmd.Flags(), &opts)
	return cmd
}
