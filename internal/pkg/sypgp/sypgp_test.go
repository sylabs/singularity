// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sypgp

import (
	"bytes"
	"encoding/hex"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/sylabs/scs-key-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

const (
	testName    = "Test Name"
	testComment = "blah"
	testEmail   = "test@test.com"
)

var testEntity *openpgp.Entity

type mockPKSLookup struct {
	code int
	el   openpgp.EntityList
}

func (ms *mockPKSLookup) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(ms.code)
	if ms.code == http.StatusOK {
		w.Header().Set("Content-Type", "application/pgp-keys")

		wr, err := armor.Encode(w, openpgp.PublicKeyType, nil)
		if err != nil {
			log.Fatalf("failed to get encoder: %v", err)
		}
		defer wr.Close()

		for _, e := range ms.el {
			if err = e.Serialize(wr); err != nil {
				log.Fatalf("failed to serialize entity: %v", err)
			}
		}
	}
}

func TestSearchPubkey(t *testing.T) {
	ms := &mockPKSLookup{}
	srv := httptest.NewTLSServer(ms)
	defer srv.Close()

	tests := []struct {
		name      string
		code      int
		el        openpgp.EntityList
		search    string
		uri       string
		authToken string
		wantErr   bool
	}{
		{"Success", http.StatusOK, openpgp.EntityList{testEntity}, "search", srv.URL, "", false},
		{"SuccessToken", http.StatusOK, openpgp.EntityList{testEntity}, "search", srv.URL, "token", false},
		{"BadURL", http.StatusOK, openpgp.EntityList{testEntity}, "search", ":", "", true},
		{"TerribleURL", http.StatusOK, openpgp.EntityList{testEntity}, "search", "terrible:", "", true},
		{"NotFound", http.StatusNotFound, nil, "search", srv.URL, "", true},
		{"Unauthorized", http.StatusUnauthorized, nil, "search", srv.URL, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms.code = tt.code
			ms.el = tt.el

			opts := []client.Option{
				client.OptBaseURL(tt.uri),
				client.OptBearerToken(tt.authToken),
				client.OptHTTPClient(srv.Client()),
			}

			if err := SearchPubkey(t.Context(), tt.search, false, opts...); (err != nil) != tt.wantErr {
				t.Fatalf("got err %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestFetchPubkey(t *testing.T) {
	ms := &mockPKSLookup{}
	srv := httptest.NewTLSServer(ms)
	defer srv.Close()

	fp := hex.EncodeToString(testEntity.PrimaryKey.Fingerprint[:])

	tests := []struct {
		name        string
		code        int
		el          openpgp.EntityList
		fingerprint string
		uri         string
		authToken   string
		wantErr     bool
	}{
		{"Success", http.StatusOK, openpgp.EntityList{testEntity}, fp, srv.URL, "", false},
		{"SuccessToken", http.StatusOK, openpgp.EntityList{testEntity}, fp, srv.URL, "token", false},
		{"NoKeys", http.StatusOK, openpgp.EntityList{}, fp, srv.URL, "token", true},
		{"TwoKeys", http.StatusOK, openpgp.EntityList{testEntity, testEntity}, fp, srv.URL, "token", true},
		{"BadURL", http.StatusOK, openpgp.EntityList{testEntity}, fp, ":", "", true},
		{"TerribleURL", http.StatusOK, openpgp.EntityList{testEntity}, fp, "terrible:", "", true},
		{"NotFound", http.StatusNotFound, nil, fp, srv.URL, "", true},
		{"Unauthorized", http.StatusUnauthorized, nil, fp, srv.URL, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms.code = tt.code
			ms.el = tt.el

			opts := []client.Option{
				client.OptBaseURL(tt.uri),
				client.OptBearerToken(tt.authToken),
				client.OptHTTPClient(srv.Client()),
			}

			el, err := FetchPubkey(t.Context(), tt.fingerprint, opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
				return
			}

			if !tt.wantErr {
				if len(el) != 1 {
					t.Fatalf("unexpected number of entities returned: %v", len(el))
				}
				for i := range tt.el {
					if fp := el[i].PrimaryKey.Fingerprint; !bytes.Equal(fp, tt.el[i].PrimaryKey.Fingerprint) {
						t.Errorf("fingerprint mismatch: %v / %v", fp, tt.el[i].PrimaryKey.Fingerprint)
					}
				}
			}
		})
	}
}

type mockPKSAdd struct {
	t       *testing.T
	keyText string
	code    int
}

func (m *mockPKSAdd) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.code != http.StatusOK {
		w.WriteHeader(m.code)
		return
	}

	if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
		m.t.Errorf("got content type %v, want %v", got, want)
	}

	if err := r.ParseForm(); err != nil {
		m.t.Fatalf("failed to parse form: %v", err)
	}
	if got, want := r.Form.Get("keytext"), m.keyText; got != want {
		m.t.Errorf("got key text %v, want %v", got, want)
	}
}

func TestPushPubkey(t *testing.T) {
	keyText, err := serializeEntity(testEntity, openpgp.PublicKeyType)
	if err != nil {
		t.Fatalf("failed to serialize entity: %v", err)
	}

	ms := &mockPKSAdd{
		t:       t,
		keyText: keyText,
	}
	srv := httptest.NewTLSServer(ms)
	defer srv.Close()

	tests := []struct {
		name      string
		uri       string
		authToken string
		code      int
		wantErr   bool
	}{
		{"Success", srv.URL, "", http.StatusOK, false},
		{"SuccessToken", srv.URL, "token", http.StatusOK, false},
		{"BadURL", ":", "", http.StatusOK, true},
		{"TerribleURL", "terrible:", "", http.StatusOK, true},
		{"Unauthorized", srv.URL, "", http.StatusUnauthorized, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms.code = tt.code

			opts := []client.Option{
				client.OptBaseURL(tt.uri),
				client.OptBearerToken(tt.authToken),
				client.OptHTTPClient(srv.Client()),
			}

			if err := PushPubkey(t.Context(), testEntity, opts...); (err != nil) != tt.wantErr {
				t.Fatalf("got err %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureDirPrivate(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	tmpdir := t.TempDir()

	cases := []struct {
		name        string
		preFunc     func(string) error
		entry       string
		expectError bool
	}{
		{
			name:        "non-existent directory",
			entry:       "d1",
			expectError: false,
		},
		{
			name:        "directory exists",
			entry:       "d2",
			expectError: false,
			preFunc: func(d string) error {
				if err := os.MkdirAll(d, 0o777); err != nil {
					return err
				}
				return os.Chmod(d, 0o777)
			},
		},
	}

	for _, tc := range cases {
		testEntry := filepath.Join(tmpdir, tc.entry)

		if tc.preFunc != nil {
			if err := tc.preFunc(testEntry); err != nil {
				t.Errorf("Unexpected failure when calling prep function: %+v", err)
				continue
			}
		}

		err := ensureDirPrivate(testEntry)
		switch {
		case !tc.expectError && err != nil:
			t.Errorf("Unexpected failure when calling ensureDirPrivate(%q): %+v", testEntry, err)

		case tc.expectError && err == nil:
			t.Errorf("Expecting failure when calling ensureDirPrivate(%q), but got none", testEntry)

		case tc.expectError && err != nil:
			t.Errorf("Expecting failure when calling ensureDirPrivate(%q), got: %+v", testEntry, err)
			continue

		case !tc.expectError && err == nil:
			// everything ok

			fi, err := os.Stat(testEntry)
			if err != nil {
				t.Errorf("Error while examining test directory %q: %+v", testEntry, err)
			}

			if !fi.IsDir() {
				t.Errorf("Expecting a directory after calling ensureDirPrivate(%q), found something else", testEntry)
			}

			if actual, expected := fi.Mode() & ^os.ModeDir, os.FileMode(0o700); actual != expected {
				t.Errorf("Expecting mode %o, got %o after calling ensureDirPrivate(%q)", expected, actual, testEntry)
			}
		}
	}
}

func getPublicKey(data string) *packet.PublicKey {
	pkt, err := packet.Read(readerFromHex(data))
	if err != nil {
		panic(err)
	}

	pk, ok := pkt.(*packet.PublicKey)
	if !ok {
		panic("expecting packet.PublicKey, got something else")
	}

	return pk
}

func TestPrintEntity(t *testing.T) {
	cases := []struct {
		name     string
		index    int
		entity   *openpgp.Entity
		expected string
	}{
		{
			name:  "RSA key",
			index: 1,
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
				Identities: map[string]*openpgp.Identity{
					"name": {
						UserId: &packet.UserId{
							Name:    "name 1",
							Comment: "comment 1",
							Email:   "email.1@example.org",
						},
					},
				},
			},
			expected: "1)  User:              name 1 (comment 1) <email.1@example.org>\n    Creation time:     2011-01-23 16:49:20 +0000 UTC\n    Fingerprint:       5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB\n    Length (in bits):  1024\n\n",
		},
		{
			name:  "DSA key",
			index: 2,
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(dsaPkDataHex),
				Identities: map[string]*openpgp.Identity{
					"name": {
						UserId: &packet.UserId{
							Name:    "name 2",
							Comment: "comment 2",
							Email:   "email.2@example.org",
						},
					},
				},
			},
			expected: "2)  User:              name 2 (comment 2) <email.2@example.org>\n    Creation time:     2011-01-28 21:05:13 +0000 UTC\n    Fingerprint:       EECE4C094DB002103714C63C8E8FBE54062F19ED\n    Length (in bits):  1024\n\n",
		},
		{
			name:  "ECDSA key",
			index: 3,
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(ecdsaPkDataHex),
				Identities: map[string]*openpgp.Identity{
					"name": {
						UserId: &packet.UserId{
							Name:    "name 3",
							Comment: "comment 3",
							Email:   "email.3@example.org",
						},
					},
				},
			},
			expected: "3)  User:              name 3 (comment 3) <email.3@example.org>\n    Creation time:     2012-10-07 17:57:40 +0000 UTC\n    Fingerprint:       9892270B38B8980B05C8D56D43FE956C542CA00B\n    Length (in bits):  1059\n\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer

			printEntity(&b, tc.index, tc.entity)

			if actual := b.String(); actual != tc.expected {
				t.Errorf("Unexpected output from printEntity: expecting %q, got %q",
					tc.expected,
					actual)
			}
		})
	}
}

func TestPrintEntities(t *testing.T) {
	entities := []*openpgp.Entity{
		{
			PrimaryKey: getPublicKey(rsaPkDataHex),
			Identities: map[string]*openpgp.Identity{
				"name": {
					UserId: &packet.UserId{
						Name:    "name 1",
						Comment: "comment 1",
						Email:   "email.1@example.org",
					},
				},
			},
		},
		{
			PrimaryKey: getPublicKey(dsaPkDataHex),
			Identities: map[string]*openpgp.Identity{
				"name": {
					UserId: &packet.UserId{
						Name:    "name 2",
						Comment: "comment 2",
						Email:   "email.2@example.org",
					},
				},
			},
		},
		{
			PrimaryKey: getPublicKey(ecdsaPkDataHex),
			Identities: map[string]*openpgp.Identity{
				"name": {
					UserId: &packet.UserId{
						Name:    "name 3",
						Comment: "comment 3",
						Email:   "email.3@example.org",
					},
				},
			},
		},
	}

	expected := "0)  User:              name 1 (comment 1) <email.1@example.org>\n    Creation time:     2011-01-23 16:49:20 +0000 UTC\n    Fingerprint:       5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB\n    Length (in bits):  1024\n\n" +
		"1)  User:              name 2 (comment 2) <email.2@example.org>\n    Creation time:     2011-01-28 21:05:13 +0000 UTC\n    Fingerprint:       EECE4C094DB002103714C63C8E8FBE54062F19ED\n    Length (in bits):  1024\n\n" +
		"2)  User:              name 3 (comment 3) <email.3@example.org>\n    Creation time:     2012-10-07 17:57:40 +0000 UTC\n    Fingerprint:       9892270B38B8980B05C8D56D43FE956C542CA00B\n    Length (in bits):  1059\n\n"

	var b bytes.Buffer

	printEntities(&b, entities)

	if actual := b.String(); actual != expected {
		t.Errorf("Unexpected output from printEntities: expecting %q, got %q",
			expected,
			actual)
	}
}

func TestGenKeyPair(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	// Prepare all the answers that GenKeyPair() is expecting.
	tests := []struct {
		name      string
		options   GenKeyPairOptions
		encrypted bool
		shallPass bool
	}{
		{
			name:      "valid case, not encrypted",
			options:   GenKeyPairOptions{Name: "teste", Email: "test@my.info", Comment: "", Password: ""},
			encrypted: false,
			shallPass: true,
		},
		{
			name:      "valid case, encrypted",
			options:   GenKeyPairOptions{Name: "teste", Email: "test@my.info", Comment: "", Password: "1234"},
			encrypted: true,
			shallPass: true,
		},
	}

	// Create a temporary directory to store the keyring
	dir := t.TempDir()

	keyring := NewHandle(dir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the input file that will act as stdin
			e, err := keyring.GenKeyPair(tt.options)
			if tt.shallPass && err != nil {
				t.Fatalf("valid case %s failed: %s", tt.name, err)
			}
			if !tt.shallPass && err == nil {
				t.Fatalf("invalid case %s succeeded", tt.name)
			}

			if e.PrivateKey.Encrypted != tt.encrypted {
				t.Fatalf("expected encrypted: %t got: %t", tt.encrypted, e.PrivateKey.Encrypted)
			}
		})
	}
}

func TestCompareKeyEntity(t *testing.T) {
	cases := []struct {
		name        string
		entity      *openpgp.Entity
		fingerprint string
		expected    bool
	}{
		{
			name: "RSA key correct fingerprint",
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
			},
			fingerprint: "5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB",
			expected:    true,
		},
		{
			name: "RSA key incorrect fingerprint",
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
			},
			fingerprint: "0FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB",
			expected:    false,
		},
		{
			name: "RSA key fingerprint too long",
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
			},
			fingerprint: "5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB00",
			expected:    false,
		},
		{
			name: "RSA key fingerprint too short",
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
			},
			fingerprint: "5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31",
			expected:    false,
		},
		{
			name: "RSA key empty fingerprint",
			entity: &openpgp.Entity{
				PrimaryKey: getPublicKey(rsaPkDataHex),
			},
			fingerprint: "",
			expected:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := compareKeyEntity(tc.entity, tc.fingerprint)

			if actual != tc.expected {
				t.Errorf("Unexpected result from compareKeyEntity: expecting %v, got %v",
					tc.expected,
					actual)
			}
		})
	}
}

func TestFindKeyByPringerprint(t *testing.T) {
	cases := []struct {
		name        string
		list        openpgp.EntityList
		fingerprint string
		exists      bool
	}{
		{
			name:        "nil list, empty needle",
			list:        nil,
			fingerprint: "",
			exists:      false,
		},
		{
			name:        "nil list, non-empty needle",
			list:        nil,
			fingerprint: "5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB",
			exists:      false,
		},
		{
			name:        "empty list, empty needle",
			list:        openpgp.EntityList{},
			fingerprint: "",
			exists:      false,
		},
		{
			name:        "empty list, non-empty needle",
			list:        openpgp.EntityList{},
			fingerprint: "5FB74B1D03B1E3CB31BC2F8AA34D7E18C20C31BB",
			exists:      false,
		},
		{
			name: "non-empty list, empty needle",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
			},
			fingerprint: "",
			exists:      false,
		},
		{
			name: "non-empty list, non-empty needle, exists",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      true,
		},
		{
			name: "non-empty list, non-empty needle, does not exist",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := findKeyByFingerprint(tc.list, tc.fingerprint)

			switch {
			case found == nil && tc.exists:
				// not found, but it exists
				t.Errorf("Searching for %q found nothing but it exists in the entity list",
					tc.fingerprint)

			case found != nil && !tc.exists:
				// found, but it does not exist
				t.Errorf("Searching for %q found %q but it should not exist in the entity list",
					tc.fingerprint,
					found.PrimaryKey.Fingerprint)

			case found == nil && !tc.exists:
				// not found, it does not exist
				return

			case found != nil && tc.exists:
				// found, it exists, is it the expected one?
				if !compareKeyEntity(found, tc.fingerprint) {
					t.Errorf("Searching for %q found %q",
						tc.fingerprint,
						found.PrimaryKey.Fingerprint)
				}
			}
		})
	}
}

func TestRemoveKey(t *testing.T) {
	cases := []struct {
		name        string
		list        openpgp.EntityList
		fingerprint string
		exists      bool
	}{
		{
			name:        "nil list, empty needle",
			list:        nil,
			fingerprint: "",
			exists:      false,
		},
		{
			name:        "empty list, empty needle",
			list:        openpgp.EntityList{},
			fingerprint: "",
			exists:      false,
		},
		{
			name:        "nil list, non-empty needle",
			list:        nil,
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      false,
		},
		{
			name:        "empty list, non-empty needle",
			list:        openpgp.EntityList{},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      false,
		},
		{
			name: "list with many elements, needle does not exist",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
			},
			fingerprint: "0892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      false,
		},
		{
			name: "list with one element, needle exists",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      true,
		},
		{
			name: "list with many elements, needle at the beginning",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      true,
		},
		{
			name: "list with many elements, needle in the middle",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      true,
		},
		{
			name: "list with many elements, needle at the end",
			list: openpgp.EntityList{
				{PrimaryKey: getPublicKey(rsaPkDataHex)},
				{PrimaryKey: getPublicKey(dsaPkDataHex)},
				{PrimaryKey: getPublicKey(ecdsaPkDataHex)},
			},
			fingerprint: "9892270B38B8980B05C8D56D43FE956C542CA00B",
			exists:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newList := removeKey(tc.list, tc.fingerprint)

			switch {
			case tc.exists && newList == nil:
				// needle does exist, no new list returned
				t.Errorf("Removing %q returned a nil list but it exists in the entity list",
					tc.fingerprint)

			case !tc.exists && newList != nil:
				// needle does not exist, new list returned
				t.Errorf("Removing %q returned a non-nil list but it does not exist in the entity list",
					tc.fingerprint)

			case !tc.exists && newList == nil:
				// needle does not exist, no new list returned
				return

			case tc.exists && newList != nil:
				// needle does exist, new list returned
				if len(newList) != len(tc.list)-1 {
					t.Errorf("After removing key %q the new list should have exactly one less element than the original, actual: %d, expected: %d",
						tc.fingerprint,
						len(newList),
						len(tc.list)-1)
				}

				if found := findKeyByFingerprint(newList, tc.fingerprint); found != nil {
					t.Errorf("After removing key %q it should not be present in the new list, but it was found there",
						tc.fingerprint)
				}
			}
		})
	}
}

func TestGlobalKeyRing(t *testing.T) {
	dir := t.TempDir()

	keypairOptions := GenKeyPairOptions{
		Name:      "test",
		Email:     "test@test.com",
		KeyLength: 2048,
	}

	keyring := NewHandle(dir, GlobalHandleOpt())

	_, err := keyring.GenKeyPair(keypairOptions)
	if err == nil {
		t.Errorf("unexpected success while generating keypair for global keyring")
	}

	_, err = keyring.genKeyPair(keypairOptions)
	if err == nil {
		t.Errorf("unexpected success while generating keypair for global keyring")
	}

	_, err = keyring.LoadPrivKeyring()
	if err == nil {
		t.Errorf("unexpected success while loading private key from global keyring")
	}

	el, err := keyring.LoadPubKeyring()
	if err != nil {
		t.Errorf("unexpected error while loading public key from global keyring: %s", err)
	} else if len(el) != 0 {
		t.Errorf("unexpected number of PGP keys: got %d instead of 0", len(el))
	}

	err = keyring.importPublicKey(&openpgp.Entity{
		PrimaryKey: getPublicKey(rsaPkDataHex),
	})
	if err != nil {
		t.Errorf("unexpected error while importing public key into global keyring: %s", err)
	}
}

func TestMain(m *testing.M) {
	// Set TZ to UTC so that the code converting a time.Time value
	// to a string produces consistent output.
	if err := os.Setenv("TZ", "UTC"); err != nil {
		log.Fatalf("Cannot set timezone: %v", err)
	}

	useragent.InitValue("singularity", "3.0.0-alpha.1-303-gaed8d30-dirty")

	e, err := openpgp.NewEntity(testName, testComment, testEmail, nil)
	if err != nil {
		log.Fatalf("failed to create entity: %v", err)
	}
	testEntity = e

	os.Exit(m.Run())
}
