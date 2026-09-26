/*
package cli provides root command that launches the tool
as well as a set of commands used in CLI mode
*/
package cli

import (
	"agywarp/internal/clash"
	"agywarp/internal/tui"
	"agywarp/internal/warp"
	"context"
	"time"

	"github.com/spf13/cobra"
)

/*
	cli/root.go
	Root command of the system
	If use alone it launches TUI service
	otherwise CLI

	Copyright 2026 (C) Ian Javik
*/

// newRoot returns the pointer of root command
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:          "agywarp",
		Short:        "launch agywarp TUI Dashboard",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}

	// add subcommands & flags if any
	root.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop an active agywarp session and clean legacy profile WARP entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
			defer cancel()
			owned, err := clash.NewManager().StopRuntime(ctx)
			if err != nil {
				return err
			}
			if owned {
				if err := warp.NewClient().Disconnect(ctx); err != nil {
					return err
				}
			}
			cmd.Println("agywarp stopped; Mihomo base and profile extensions cleaned")
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "recover",
		Short: "Restore clean Mihomo config after an interrupted agywarp session",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 40*time.Second)
			defer cancel()
			if err := clash.NewManager().RecoverRuntime(ctx); err != nil {
				return err
			}
			cmd.Println("Mihomo base config restored; WARP connection left unchanged")
			return nil
		},
	})

	return root // pointer
}

// Execute is expected to be called in main function to trigger root command
func Execute() error {
	return newRoot().Execute()
}
