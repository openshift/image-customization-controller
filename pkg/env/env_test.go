package env

import (
	"testing"
)

func TestRegistriesConf(t *testing.T) {
	inputs := EnvInputs{
		RegistriesConfPath: "../../test/registries.conf",
	}

	registries := `[[registry]]
  prefix = ""
  location = "quay.io/openshift-release-dev/ocp-v4.0-art-dev"
  mirror-by-digest-only = true

  [[registry.mirror]]
    location = "virthost.ostest.test.metalkube.org:5000/localimages/local-release-image"
`

	data, err := inputs.RegistriesConf()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if string(data) != registries {
		t.Fatalf("Registries data:\n%s\ndoes not match expected:\n%s", string(data), registries)
	}
}
