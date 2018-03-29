/*
  Copyright (c) 2018, Sylabs, Inc. All rights reserved.

  This software is licensed under a 3-clause BSD license.  Please
  consult LICENSE file distributed with the sources of this project regarding
  your rights to use or distribute this software.
*/
package cli

import (
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/singularityware/singularity/pkg/build"
	"github.com/spf13/cobra"
)

var (
	Remote      bool
	RemoteURL   string
	Sandbox     bool
	Writable    bool
	Force       bool
	NoTest      bool
	Sections    []string
    MakeManPage bool
    ManPageDir string
)

func init() {
	BuildCmd.Flags().SetInterspersed(false)
	singularityCmd.AddCommand(BuildCmd)

	BuildCmd.Flags().BoolVarP(&Sandbox, "sandbox", "s", false, "Build image as sandbox format (chroot directory structure)")
	BuildCmd.Flags().StringSliceVar(&Sections, "section", []string{}, "Only run specific section(s) of deffile")
	BuildCmd.Flags().BoolVarP(&Writable, "writable", "w", false, "Build image as writable (SIF with writable internal overlay)")
	BuildCmd.Flags().BoolVarP(&Force, "force", "f", false, "")
	BuildCmd.Flags().BoolVarP(&NoTest, "notest", "T", false, "")
	BuildCmd.Flags().BoolVarP(&Remote, "remote", "r", false, "Build image remotely")
	BuildCmd.Flags().StringVar(&RemoteURL, "remote-url", "localhost:5050", "Specify the URL of the remote builder")
}

// BuildCmd represents the build command
var BuildCmd = &cobra.Command{
    Use: "build [local options...] <image path> <build spec>",
	Args: cobra.ExactArgs(2),
    Short: "build is the shiz",
    Long: "no really, it's the fashizzle",
	Run: func(cmd *cobra.Command, args []string) {
		var def build.Definition
		var b build.Builder
		var err error

		if silent {
			fmt.Println("Silent!")
		}

		if Sandbox {
			fmt.Println("Sandbox!")
		}

		if ok, err := build.IsValidURI(args[1]); ok && err == nil {
			// URI passed as arg[1]
			def, err = build.NewDefinitionFromURI(args[1])
			if err != nil {
				glog.Error(err)
				return
			}
		} else if !ok && err == nil {
			// Non-URI passed as arg[1]
			defFile, err := os.Open(args[1])
			if err != nil {
				glog.Error(err)
				return
			}

			def, err = build.ParseDefinitionFile(defFile)
			if err != nil {
				glog.Error(err)
				return
			}
		} else {
			// Error
			glog.Error(err)
			return
		}

		if Remote {
			b = build.NewRemoteBuilder(args[0], def, false, RemoteURL)

		} else {
			b, err = build.NewSifBuilder(args[0], def)
			if err != nil {
				glog.Error(err)
				return
			}
		}

		b.Build()

		/*
			if Remote {
				doRemoteBuild(args[0], args[1])
			} else {
				if ok, err := build.IsValidURI(args[1]); ok && err == nil {
					u := strings.SplitN(args[1], "://", 2)
					b, err := build.NewSifBuilderFromURI(args[0], args[1])
					if err != nil {
						glog.Errorf("Image build system encountered an error: %s\n", err)
						return
					}
					b.Build()
				} else {
					glog.Fatalf("%s", err)
				}
			}*/

	},
	TraverseChildren: true,
}
