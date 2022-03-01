package ignition

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateStructure(t *testing.T) {
	builder, err := New(nil, nil,
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "")
	assert.NoError(t, err)

	ignition, err := builder.generate()
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Systemd.Units, 1)
	assert.Len(t, ignition.Storage.Files, 2)
	assert.Len(t, ignition.Passwd.Users, 0)

	// Sanity-check only
	assert.Contains(t, *ignition.Systemd.Units[0].Contents, "ironic-agent")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "ironic.example.com")
	assert.Equal(t, ignition.Storage.Files[1].Path, "/etc/NetworkManager/conf.d/clientid.conf")
}

func TestGenerateWithMerge(t *testing.T) {
	builder, err := New(nil, nil,
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "")
	assert.NoError(t, err)

	mergeWith := []byte(`
{
    "ignition": {
	"version": "3.1.0"
    },
    "storage": {
	"files": [
	    {
		"path": "/etc/motd",
		"mode": 420,
		"contents": {"source": "data:,Hello%20World"}
	    }
	]
    }
}
`)
	assert.True(t, json.Valid(mergeWith)) // sanity-check

	ignition, err := builder.generateAndMergeWith(mergeWith)
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Storage.Files, 3)

	assert.Equal(t, ignition.Storage.Files[0].Path, "/etc/motd")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "Hello")
	assert.Contains(t, *ignition.Storage.Files[1].Contents.Source, "ironic.example.com")
	assert.Equal(t, ignition.Storage.Files[2].Path, "/etc/NetworkManager/conf.d/clientid.conf")

	// Verify idempotancy, i.e. that we can merge our config with an
	// already merged version and get the same result.
	merged, err := builder.GenerateAndMergeWith(mergeWith)
	assert.NoError(t, err)
	assert.Contains(t, string(merged), "motd")
	assert.Contains(t, string(merged), "ironic.example.com")

	ignition, err = builder.generateAndMergeWith(merged)
	assert.NoError(t, err)

	assert.Equal(t, "3.2.0", ignition.Ignition.Version)
	assert.Len(t, ignition.Storage.Files, 3)

	assert.Equal(t, ignition.Storage.Files[0].Path, "/etc/motd")
	assert.Contains(t, *ignition.Storage.Files[0].Contents.Source, "Hello")
	assert.Contains(t, *ignition.Storage.Files[1].Contents.Source, "ironic.example.com")
	assert.Equal(t, ignition.Storage.Files[2].Path, "/etc/NetworkManager/conf.d/clientid.conf")
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
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", "", "", "", "virthost")
	assert.NoError(t, err)

	ignition, err := builder.Generate()
	assert.NoError(t, err)

	registriesData := "\"data:text/plain,%0A%5B%5Bregistry%5D%5D%0A%20%20prefix%20%3D%20%22%22%0A%20%20location%20%3D%20%22quay.io%2Fopenshift-release-dev%2Focp-v4.0-art-dev%22%0A%20%20mirror-by-digest-only%20%3D%20true%0A%0A%20%20%5B%5Bregistry.mirror%5D%5D%0A%20%20%20%20location%20%3D%20%22virthost.ostest.test.metalkube.org%3A5000%2Flocalimages%2Flocal-release-image%22%0A\""
	assert.Contains(t, string(ignition), registriesData, "registries data not found in ignition")
}
