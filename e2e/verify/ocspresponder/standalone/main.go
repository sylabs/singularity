package main

import (
	"fmt"
	"os"

	"github.com/sylabs/singularity/e2e/verify/ocspresponder"
)

func main() {
	if err := ocspresponder.StartOCSPResponder(ocspresponder.DefaultOCSPResponderArgs); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
