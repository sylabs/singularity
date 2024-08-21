package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/app/singularity"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var (
	overlaySize   int
	overlayDirs   []string
	overlaySparse bool
)

// -s|--size
var overlaySizeFlag = cmdline.Flag{
	ID:           "overlaySizeFlag",
	Value:        &overlaySize,
	DefaultValue: 64,
	Name:         "size",
	ShortHand:    "s",
	Usage:        "size of the EXT3 writable overlay in MiB",
}

// --sparse/-S
var overlaySparseFlag = cmdline.Flag{
	ID:           "overlaySparseFlag",
	Value:        &overlaySparse,
	DefaultValue: false,
	Name:         "sparse",
	ShortHand:    "S",
	Usage:        "create a sparse overlay",
	EnvKeys:      []string{"SPARSE"},
}

// --create-dir
var overlayCreateDirFlag = cmdline.Flag{
	ID:           "overlayCreateDirFlag",
	Value:        &overlayDirs,
	DefaultValue: []string{},
	Name:         "create-dir",
	Usage:        "directory to create as part of the overlay layout",
}

// OverlayCreateCmd is the 'overlay create' command that allows to create writable overlay.
var OverlayCreateCmd = &cobra.Command{
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if err := singularity.OverlayCreate(args[0], overlaySize, overlaySparse, overlayDirs...); err != nil {
			sylog.Fatalf("%v", err.Error())
		}
		return nil
	},
	DisableFlagsInUseLine: true,

	Use:     docs.OverlayCreateUse,
	Short:   docs.OverlayCreateShort,
	Long:    docs.OverlayCreateLong,
	Example: docs.OverlayCreateExample,
}
