{
	"cdiVersion": "0.5.0",
	"kind": "singularityCEtesting.sylabs.io/device",

	"devices": [
		{
			"name": "e2eMountTester",
			"containerEdits": {
				"deviceNodes": {{tojson .DeviceNodes}},
				"mounts": {{tojson .Mounts}}
			}
		}
	],

	"containerEdits": {
		"env": {{tojson .Env}}
	}
}
