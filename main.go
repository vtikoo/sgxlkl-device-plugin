package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT}
)

// Execute runs the cobra cmd
func Execute() {
	if err := runCmd.Execute(); err != nil {
		glog.Fatal(err)
		os.Exit(1)
	}
}

var runCmd = &cobra.Command{
	Use:   "sgxlkl-device-plugin",
	Short: "sgxlkl-device-plugin is SGX-LKL device plugin",
	Long:  "sgxlkl-device-plugin is SGX-LKL device plugin",
	PreRun: func(cmd *cobra.Command, args []string) {
		printConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		var mgr *SGXLKLManager
		var err error
		reset := true

		c := make(chan os.Signal, 5)
		signal.Notify(c, shutdownSignals...)

		for {
			if reset {
				if mgr != nil {
					mgr.Stop()
				}
				mgr, err = NewSGXLKLManager()
				if err != nil {
					glog.Errorf("cannot create SGX-LKL device manager: %v", err)
					glog.Errorf("Waiting indefinitely...")
					reset = false
					continue
				}

				err = mgr.Run()
				if err != nil {
					glog.Errorf("command returned err: %v", err)
					os.Exit(1)
				}
				reset = false
			}

			select {
			case s := <-c:
				switch s {
				case syscall.SIGHUP:
					glog.Infof("Received SIGHUP signal, reseting...")
					reset = true
				default:
					glog.Infof("received %v signal, stopping...", s)
					mgr.Stop()
					os.Exit(1)
				}
			}
		}
	},
}

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	err := pflag.Set("logtostderr", "true")
	if err != nil {
		glog.Errorf("failed to init, err: %v", err)
		os.Exit(1)
	}
	runCmd.Flags().AddGoFlagSet(flag.CommandLine)
}

func printConfig() {

}

func main() {
	runCmd.Execute()
}
