# Dependency Licenses

This project uses a number of dependencies, in accordance with their own
license terms. These dependencies are managed via the project `go.mod`
and `go.sum` files, and included in a `vendor/` directory in our official
source tarballs.

A full build or package of SingularityCE uses all dependencies listed below.
If you `import "github.com/sylabs/singularity/v4"` into your own project then
you may use a subset of them.

The dependencies and their licenses are as follows:

{{ range . }}

## {{ .Name }}

**License:** {{ .LicenseName }}

```
{{ .LicenseText }}
```
{{ end }}
