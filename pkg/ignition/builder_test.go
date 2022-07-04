package ignition

import (
	"strings"
	"testing"
)

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
