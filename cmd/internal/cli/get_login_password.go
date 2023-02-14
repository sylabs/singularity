package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/singularity/internal/pkg/client/library"
	"github.com/sylabs/singularity/pkg/cmdline"
	"github.com/sylabs/singularity/pkg/sylog"
)

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(GetLoginPasswordCmd)
	})
}

var GetLoginPasswordCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExatArgs(1),
	// figure out how to call oci library api call
	// below is code from search feature
	Run: func(cmd *cobra.Command, args []string) {
		config, err := getLibraryClientConfig(SearchLibraryURI)
		if err != nil {
			sylog.Fatalf("Error while getting library client config: %v", err)
		}

		libraryClient, err := client.NewClient(config)
		if err != nil {
			sylog.Fatalf("Error initializing library client: %v", err)
		}

		if err := library.SearchLibrary(cmd.Context(), libraryClient, args[0], SearchArch, SearchSigned); err != nil {
			sylog.Fatalf("Couldn't search library: %v", err)
		}

	},

	Use:     "",
	Short:   "",
	Long:    "",
	Example: "",
}
