package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
)

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(OverlayCmd)

		cmdManager.RegisterSubCmd(OverlayCmd, OverlayCreateCmd)
		cmdManager.RegisterFlagForCmd(&overlaySizeFlag, OverlayCreateCmd)
		cmdManager.RegisterFlagForCmd(&overlayCreateDirFlag, OverlayCreateCmd)
		cmdManager.RegisterFlagForCmd(&overlaySparseFlag, OverlayCreateCmd)

		cmdManager.RegisterSubCmd(OverlayCmd, OverlaySyncCmd)

		cmdManager.RegisterSubCmd(OverlayCmd, OverlaySealCmd)
	})
}

// OverlayCmd is the 'overlay' command that allows to manage writable overlay.
var OverlayCmd = &cobra.Command{
	RunE: func(_ *cobra.Command, _ []string) error {
		return errors.New("Invalid command")
	},
	DisableFlagsInUseLine: true,

	Use:     docs.OverlayUse,
	Short:   docs.OverlayShort,
	Long:    docs.OverlayLong,
	Example: docs.OverlayExample,
}
