package ignition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vincent-petithory/dataurl"
	"k8s.io/client-go/util/cert"
)

func TestGenerateStructure(t *testing.T) {
	builder, err := New(nil, nil,
		"http://ironic.example.com", "",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "", "", []string{}, "", "")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Systemd.Units, 1)
	assert.Len(t, ignition.Storage.Files, 4)
	assert.Len(t, ignition.Passwd.Users, 0)

	// Sanity-check only
	assert.Contains(t, *ignition.Systemd.Units[0].Contents, "ironic-agent")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A6385")
	assert.NotContains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A5050")
	assert.Equal(t, ignition.Storage.Files[1].Path, "/etc/NetworkManager/conf.d/clientid.conf")
	assert.Equal(t, ignition.Storage.Files[2].Path, "/etc/issue.d/ipa.issue")
	assert.Equal(t, ignition.Storage.Files[3].Path, "/etc/motd.d/ipa.motd")
}

func TestGenerateWithMoreFields(t *testing.T) {
	file, _ := os.CreateTemp("/tmp/", "icc_")
	defer os.Remove(file.Name())
	builder, err := New(nil, []byte("I am registry"),
		"http://ironic.example.com", "http://inspector.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"pull secret", "SSH key", "ip=dhcp42",
		"proxy me", "", "don't proxy me", "my-host", "", []string{}, file.Name(), "")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Systemd.Units, 1)
	assert.Len(t, ignition.Storage.Files, 8)
	assert.Len(t, ignition.Passwd.Users, 1)

	// Sanity-check only
	assert.Contains(t, *ignition.Systemd.Units[0].Contents, "ironic-agent")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A6385")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "inspector.example.com%3A5050")
	assert.Equal(t, ignition.Storage.Files[1].Path, "/etc/authfile.json")
	assert.Equal(t, ignition.Storage.Files[2].Path, "/etc/pki/ca-trust/source/anchors/ca.crt")
	assert.Equal(t, ignition.Storage.Files[3].Path, "/etc/NetworkManager/conf.d/clientid.conf")
	assert.Equal(t, ignition.Storage.Files[4].Path, "/etc/NetworkManager/dispatcher.d/01-hostname")
	assert.Equal(t, ignition.Storage.Files[5].Path, "/etc/containers/registries.conf")
	assert.Equal(t, ignition.Storage.Files[6].Path, "/etc/issue.d/ipa.issue")
	assert.Equal(t, ignition.Storage.Files[7].Path, "/etc/motd.d/ipa.motd")
	assert.Equal(t, ignition.Passwd.Users[0].Name, "core")
	assert.Len(t, ignition.Passwd.Users[0].SSHAuthorizedKeys, 1)
}

func TestGenerateIronicCABundle(t *testing.T) {
	caData, _, err := cert.GenerateSelfSignedCertKey("ironic.example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ironic-ca.crt")
	if err := os.WriteFile(caFile, caData, 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		caFile    string
		wantError bool
	}{
		{name: "CA embedded", caFile: caFile},
		{name: "no CA"},
		{name: "missing CA", caFile: filepath.Join(t.TempDir(), "missing.crt"), wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := New(nil, nil,
				"https://ironic.example.com", "",
				"quay.io/openshift-release-dev/ironic-ipa-image",
				"", "", "", "", "", "", "", "", nil, "", tt.caFile)
			if err != nil {
				t.Fatal(err)
			}
			config, err := builder.GenerateConfig()
			if tt.wantError {
				assert.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			files := make(map[string]string)
			for _, file := range config.Storage.Files {
				if !assert.NotNil(t, file.Contents.Source) {
					t.FailNow()
				}
				data, err := dataurl.DecodeString(*file.Contents.Source)
				if err != nil {
					t.Fatal(err)
				}
				files[file.Path] = string(data.Data)
				if file.Path == ironicCaBundlePath {
					if assert.NotNil(t, file.Mode) {
						assert.Equal(t, 0644, *file.Mode)
					}
				}
			}
			assert.Contains(t, files, "/etc/ironic-python-agent.conf")
			if !assert.Len(t, config.Systemd.Units, 1) || !assert.NotNil(t, config.Systemd.Units[0].Contents) {
				t.FailNow()
			}
			service := *config.Systemd.Units[0].Contents
			if tt.caFile == "" {
				assert.NotContains(t, files, ironicCaBundlePath)
				assert.Contains(t, files["/etc/ironic-python-agent.conf"], "insecure = True")
				assert.NotContains(t, service, ironicCaBundlePath)
				assert.NotContains(t, service, "REQUESTS_CA_BUNDLE")
				assert.NotContains(t, service, "SSL_CERT_FILE")
			} else {
				assert.Equal(t, string(caData), files[ironicCaBundlePath])
				assert.Contains(t, files["/etc/ironic-python-agent.conf"], "insecure = False")
				assert.Contains(t, service, "--mount type=bind,src="+ironicCaBundlePath+",dst="+ironicCaBundlePath+",ro")
				assert.Contains(t, service, "--env REQUESTS_CA_BUNDLE="+ironicCaBundlePath)
				assert.Contains(t, service, "--env SSL_CERT_FILE="+ironicCaBundlePath)
			}
		})
	}
}

func TestGenerateRegistries(t *testing.T) {
	registries := `
[[registry]]
  prefix = ""
  location = "quay.io/openshift-release-dev/ocp-v4.0-art-dev"
  mirror-by-digest-only = true

  [[registry.mirror]]
    location = "virthost.ostest.test.metalkube.org:5000/localimages/local-release-image"
`
	builder, err := New([]byte{}, []byte(registries),
		"http://ironic.example.com", "",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "virthost", "", []string{}, "", "")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	ignition, err := builder.Generate()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	registriesData := "\"data:text/plain,%0A%5B%5Bregistry%5D%5D%0A%20%20prefix%20%3D%20%22%22%0A%20%20location%20%3D%20%22quay.io%2Fopenshift-release-dev%2Focp-v4.0-art-dev%22%0A%20%20mirror-by-digest-only%20%3D%20true%0A%0A%20%20%5B%5Bregistry.mirror%5D%5D%0A%20%20%20%20location%20%3D%20%22virthost.ostest.test.metalkube.org%3A5000%2Flocalimages%2Flocal-release-image%22%0A\""
	if !strings.Contains(string(ignition), registriesData) {
		t.Fatalf("Registries data not found in ignition:\n%s", string(ignition))
	}
}

func TestGenerateIPAIdentificationFiles(t *testing.T) {
	builder, err := New(nil, nil,
		"http://ironic.example.com", "",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "", "", []string{}, "", "")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	// Find the issue file
	var issueFile *string
	var motdFile *string
	for _, file := range ignition.Storage.Files {
		if file.Path == "/etc/issue.d/ipa.issue" {
			issueFile = file.Contents.Source
		}
		if file.Path == "/etc/motd.d/ipa.motd" {
			motdFile = file.Contents.Source
		}
	}

	// Verify issue file exists and contains IPA identification
	assert.NotNil(t, issueFile, "Issue file should be present")
	assert.Contains(t, *issueFile, "Ironic%20Python%20Agent")
	assert.Contains(t, *issueFile, "discovery%20image")

	// Verify MOTD file exists and contains IPA identification
	assert.NotNil(t, motdFile, "MOTD file should be present")
	assert.Contains(t, *motdFile, "Ironic%20Python%20Agent")
	assert.Contains(t, *motdFile, "discovery%20image")
	// Verify debugging info is included
	assert.Contains(t, *motdFile, "ironic.example.com")
	assert.Contains(t, *motdFile, "quay.io")
	assert.Contains(t, *motdFile, "journalctl")
}

func TestGenerateIPAIdentificationWithDebuggingInfo(t *testing.T) {
	// Test without NMState data to avoid requiring nmstatectl in CI
	builder, err := New(nil, nil,
		"http://ironic.example.com", "http://inspector.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image:v4.17",
		"", "", "", "", "", "", "my-hostname", "", []string{}, "", "")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	// Find the MOTD file
	var motdFile *string
	for _, file := range ignition.Storage.Files {
		if file.Path == "/etc/motd.d/ipa.motd" {
			motdFile = file.Contents.Source
		}
	}

	// Verify MOTD file contains debugging information
	assert.NotNil(t, motdFile, "MOTD file should be present")
	assert.Contains(t, *motdFile, "ironic.example.com", "Should contain Ironic URL")
	assert.Contains(t, *motdFile, "inspector.example.com", "Should contain Inspector URL")
	assert.Contains(t, *motdFile, "my-hostname", "Should contain hostname")
	assert.Contains(t, *motdFile, "quay.io%2Fopenshift-release-dev%2Fironic-ipa-image", "Should contain IPA image")
	assert.Contains(t, *motdFile, "Useful%20commands", "Should contain useful commands section")
	assert.Contains(t, *motdFile, "journalctl", "Should contain journalctl command")
	// Without NMState data, should not mention custom network config
	assert.NotContains(t, *motdFile, "Custom%20NMState", "Should not mention NMState when not configured")
}
