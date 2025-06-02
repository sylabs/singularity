// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package progress

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/sylabs/singularity/v4/pkg/sylog"
)

func TestProgressCallback(t *testing.T) {
	const input = "Hello World!"
	ctx := t.Context()

	// Check the progress bar, or invisible copy-through, works at all sylog
	// levels

	levels := []int{
		int(sylog.DebugLevel),
		int(sylog.VerboseLevel),
		int(sylog.InfoLevel),
		int(sylog.WarnLevel),
		int(sylog.ErrorLevel),
		int(sylog.FatalLevel),
	}

	for _, l := range levels {
		t.Run(fmt.Sprintf("level%d", l), func(t *testing.T) {
			sylog.SetLevel(l, true)

			cb := BarCallback(ctx)
			src := bytes.NewBufferString(input)
			dst := bytes.Buffer{}

			err := cb(int64(len(input)), src, &dst)
			if err != nil {
				t.Errorf("Unexpected error from ProgressCallBack: %v", err)
			}

			output := dst.String()
			if output != input {
				t.Errorf("Output from callback '%s' != input '%s'", output, input)
			}
		})
	}
}
