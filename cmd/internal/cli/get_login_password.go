package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/internal/pkg/remote/endpoint"
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
	Run: func(cmd *cobra.Command, args []string) {
		// make a request to the shim api
		ep := "https://library.se.k3s/v1/rbac/users/current"
		authToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL3Rva2VuLnNlLmszcyIsInN1YiI6IjYzZGFkYjliZjMwMzEzMzE3YmI2ODc5YSIsImV4cCI6MTY3OTM0NjQxOCwiaWF0IjoxNjc2NzU0NDE4LCJqdGkiOiI2M2YxM2RmMmYyY2M1MTZkYzUwNGUwMmUifQ.fA2jn5x4YWJXib3fKHY1Qc6qp8D4If0GAY_K4Na0J7F_cY0JnY0irPErTb2ttLV683-QmgHopqz_DGmxzde5vxzoKCjMf1BJSO5WoFj5TEAcaiIy97V8n0yBgWpEbEySjhmcEFI5kJGDRKKUViPNj7sY1cus2owpMf9iuteO3IC_EPnjaFGk4RUuNVqdf8glioWK70Fy6ycBbuNj5_ldJnIhgl47ra2xVBFUs9lBCTk35WZRoLZlnHUqAP_0h3l7EHQsFm0ljjNiWY28UMkI3XxCrI6erUdAgPdPJEoUEICEVl9sPj8ZLa5n83dy1PB6MMZD49SYU3HUBuXNPVwJqA"
		req, err := http.NewRequest("GET", ep, nil)
		if err != nil {
			fmt.Errorf("request err: ", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", authToken))
		client := http.DefaultClient
		res, err := client.Do(req)
		if err != nil {
			fmt.Errorf("client err: %v", err)
		}
		var u User
		err = json.NewDecoder(res.Body).Decode(&u)
		if err != nil {
			fmt.Errorf("jsonerr: %v", err)
		}
		if u.OidcUserMeta.Secret != "" {
			fmt.Println(u.OidcUserMeta.Secret)
		} else {
			fmt.Errorf("failed to get secret: %v", err)
		}

		//harborURI := "https://harbor.se.k3s/api/v2.0/"
		// Make a config to use shim api for base URL
		//_, authToken, _ := getClientConfig(harborURI)
		// hit the harbor api and decode the json resp
		// I might need to se the User Agent Header ????
		// decode json resp

	},

	Use:     "get-login-password",
	Short:   "",
	Long:    "",
	Example: "",
}

func getClientConfig(uri string) (baseURI, authToken string, err error) {
	if currentRemoteEndpoint == nil {
		var err error

		// if we can load config and if default endpoint is set, use that
		// otherwise fall back on regular authtoken and URI behavior
		currentRemoteEndpoint, err = sylabsRemote()
		if err != nil {
			return "", "", fmt.Errorf("unable to load remote configuration: %v", err)
		}
	}
	if currentRemoteEndpoint == endpoint.DefaultEndpointConfig {
		sylog.Warningf("No default remote in use, falling back to default builder: %s", endpoint.SCSDefaultBuilderURI)
	}

	return currentRemoteEndpoint.BuilderClientConfig(uri)
}

type OidcUserMeta struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Subiss       string `json:"subiss"`
	Secret       string `json:"secret"`
	CreationTime string `json:"creation_time"`
	UpdateTime   string `json:"update_time"`
}

// User
type User struct {
	Email           string       `json:"email"`
	RealName        string       `json:"realname"`
	Comment         string       `json:"comment"`
	UserId          string       `json:"user_id"`
	UserName        string       `json:"username"`
	SysAdminFlag    bool         `json:"sysadmin_flag"`
	AdminRoleInAuth string       `json:"admin_role_in_auth"`
	OidcUserMeta    OidcUserMeta `json:"oidc_user_meta"`
	CreationTime    string       `json:"creation_time"`
	UpdateTime      string       `json:"update_time"`
}
