package ignition

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateStructure(t *testing.T) {
	builder, err := New(nil, nil,
		"http://ironic.example.com", "",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Systemd.Units, 2)
	assert.Len(t, ignition.Storage.Files, 2)
	assert.Len(t, ignition.Passwd.Users, 0)

	// Sanity-check only
	assert.Contains(t, *ignition.Systemd.Units[0].Contents, "ironic-agent")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A6385")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A5050")
	assert.Equal(t, ignition.Storage.Files[1].Path, "/etc/NetworkManager/conf.d/clientid.conf")
}

func TestGenerateWithMoreFields(t *testing.T) {
	builder, err := New(nil, []byte("I am registry"),
		"http://ironic.example.com", "http://inspector.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"pull secret", "SSH key", "ip=dhcp42",
		"proxy me", "", "don't proxy me", "my-host")
	assert.NoError(t, err)

	ignition, err := builder.GenerateConfig()
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Systemd.Units, 2)
	assert.Len(t, ignition.Storage.Files, 5)
	assert.Len(t, ignition.Passwd.Users, 1)

	// Sanity-check only
	assert.Contains(t, *ignition.Systemd.Units[0].Contents, "ironic-agent")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com%3A6385")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "inspector.example.com%3A5050")
	assert.Equal(t, ignition.Storage.Files[1].Path, "/etc/authfile.json")
	assert.Equal(t, ignition.Storage.Files[2].Path, "/etc/NetworkManager/conf.d/clientid.conf")
	assert.Equal(t, ignition.Storage.Files[3].Path, "/etc/NetworkManager/dispatcher.d/01-hostname")
	assert.Equal(t, ignition.Storage.Files[4].Path, "/etc/containers/registries.conf")
	assert.Equal(t, ignition.Passwd.Users[0].Name, "core")
	assert.Len(t, ignition.Passwd.Users[0].SSHAuthorizedKeys, 1)
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
		"", "", "", "", "", "", "virthost")
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
