// Copyright (c) 2019-2026 Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	bmanclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildmanager/client"
	bmanops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildmanager/client/operations"
	bsrvclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildserver/client"
	bsrvops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildserver/client/operations"
	consentclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client"
	consentbase "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client/base"
	keyclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/keyservice/client"
	keyops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/keyservice/client/operations"
	libraryclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client"
	librarybase "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/base"
	tokenclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/tokenservice/client"
	tokenops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/tokenservice/client/operations"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

type status struct {
	name    string
	status  string
	version string
}

func (s status) str() string {
	return strings.Join([]string{s.name, s.status, s.version}, "\t")
}

// initStatus sets up the status command which is now independent of Singularity code.
func (e *Executor) initStatus() *cobra.Command {
	cmd := cobra.Command{
		Run: func(cmd *cobra.Command, _ []string) {
			remoteStatus(cmd, e.clients, e.authInfo)
		},

		Use:   `status`,
		Short: `Check the status of the Singularity Enterprise services, and your authentication token`,
		Long: `
	The 'enterprise status' command checks the status of the specified remote endpoint
	and reports the availability of services and their versions. If no endpoint is
	 configured, it will check the status of the default config (SylabsCloud). If you
	 have logged in with an authentication token the validity of that token will be
	 checked.`,
		Example: `singularity enterprise status`,

		DisableFlagsInUseLine: true,
	}
	return &cmd
}

func remoteStatus(cmd *cobra.Command, clients *api.Clients, authInfo runtime.ClientAuthInfoWriter) {
	ls, err := libraryStatus(clients.LibraryClient)
	if err != nil {
		sylog.Errorf("While checking library status: %v", err)
	}
	bss, err := buildServerStatus(clients.BuildSrvClient)
	if err != nil {
		sylog.Errorf("While checking build server status: %v", err)
	}
	bms, err := buildManagerStatus(clients.BuildManClient)
	if err != nil {
		sylog.Errorf("While checking build server status: %v", err)
	}
	ks, err := keyServiceStatus(clients.KeyClient)
	if err != nil {
		sylog.Errorf("While checking key service status: %v", err)
	}
	cs, err := consentServiceStatus(clients.ConsentClient)
	if err != nil {
		sylog.Errorf("While checking key service status: %v", err)
	}
	ts, err := tokenServiceStatus(clients.TokenClient)
	if err != nil {
		sylog.Errorf("While checking key service status: %v", err)
	}

	baseURI, _ := cmd.PersistentFlags().GetString("uri")
	cmd.Printf("\nStatus for Enterprise installation %s\n\n", baseURI)

	status := []status{ls, bss, bms, ks, cs, ts}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tVERSION")
	for _, s := range status {
		fmt.Fprintln(w, s.str())
	}
	w.Flush()

	if authInfo != nil {
		cmd.Println("\n" + tokenStatus(clients.TokenClient, authInfo))
	} else {
		cmd.Println("\nNo authentication token provided")
	}
}

func libraryStatus(lc *libraryclient.SylabsContainerLibraryAPIOCISHIM) (s status, err error) {
	resp, err := lc.Base.GetVersion(librarybase.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "library"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func buildServerStatus(bsc *bsrvclient.SylabsRemoteBuildServer) (s status, err error) {
	resp, err := bsc.Operations.GetVersion(bsrvops.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "build server"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func buildManagerStatus(bmc *bmanclient.SylabsRemoteBuildManager) (s status, err error) {
	resp, err := bmc.Operations.GetVersion(bmanops.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "build manager"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func keyServiceStatus(kc *keyclient.SylabsKeyServiceAPI) (s status, err error) {
	resp, err := kc.Operations.GetVersion(keyops.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "key service"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func consentServiceStatus(cc *consentclient.SylabsConsentServiceAPI) (s status, err error) {
	resp, err := cc.Base.GetVersion(consentbase.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "consent service"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func tokenServiceStatus(tc *tokenclient.SylabsTokenServiceAPI) (s status, err error) {
	resp, err := tc.Operations.GetVersion(tokenops.NewGetVersionParams())
	if err != nil {
		s.status = "ERROR"
		return s, err
	}

	s.name = "token service"
	s.status = "OK"
	s.version = resp.Payload.Data.Version
	return s, nil
}

func tokenStatus(tc *tokenclient.SylabsTokenServiceAPI, authInfo runtime.ClientAuthInfoWriter) string {
	_, err := tc.Operations.TokenStatus(tokenops.NewTokenStatusParams(), authInfo)
	if err != nil {
		return "Auth token is not valid"
	}
	return "Auth token is valid"
}
