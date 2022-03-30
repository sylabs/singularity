module github.com/sylabs/singularity/e2e-cli-plugin

go 1.17

require (
	github.com/spf13/cobra v1.4.0
	github.com/sylabs/singularity v0.0.0
)

require (
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace github.com/sylabs/singularity => ./singularity_source
