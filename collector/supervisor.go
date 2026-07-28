package main

import (
	"fmt"

	// TODO: this imported to just download dependencies. Bare imports will be removed after newOtelSupervisorCommand is finished.
	_ "github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor"
	_ "github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor/config"
	_ "github.com/open-telemetry/opentelemetry-collector-contrib/cmd/opampsupervisor/supervisor/telemetry"
	_ "go.opentelemetry.io/collector/confmap"

	"github.com/spf13/cobra"
)

func newOtelSupervisorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "supervisor",
		Aliases: []string{"sv"},
		Short:   "Run the embedded OpAMP supervisor for Alloy's OTel engine",
		Long:    "[EXPERIMENTAL] Run an embedded OpAMP supervisor that manages alloy as a supervised agent",
		Example: "<example comes here>",
		// PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 	panic("TODO")
		// },
		// PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 	panic("TODO")
		// },
		// PreRun: func(cmd *cobra.Command, args []string) {
		// 	panic("TODO")
		// },
		// PreRunE: func(cmd *cobra.Command, args []string) error {
		// 	panic("TODO")
		// },
		// Run: func(cmd *cobra.Command, args []string) {
		// 	panic("TODO")
		// },
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented yet")
		},
		// PostRun: func(cmd *cobra.Command, args []string) {
		// 	panic("TODO")
		// },
		// PostRunE: func(cmd *cobra.Command, args []string) error {
		// 	panic("TODO")
		// },
		// PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// 	panic("TODO")
		// },
		// PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// 	panic("TODO")
		// },
		TraverseChildren: false,
		Hidden:           false,
		SilenceErrors:    false,
		SilenceUsage:     true,
	}
	return cmd
}
