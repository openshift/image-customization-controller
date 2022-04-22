package ignition

import (
	"strings"
	"testing"

	"github.com/openshift/image-customization-controller/pkg/env"
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
		"", "", "", env.ProxyConfig{}, "virthost")
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

func TestDefaultEnv(t *testing.T) {
	builder, _ := New([]byte{}, []byte{},
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", env.ProxyConfig{}, "virthost")

	envConfig := builder.defaultEnv()
	if string(builder.defaultEnv()) != "[Manager]\n" {
		t.Errorf("Unexpected default env file:\n%s", envConfig)
	}

	builder, _ = New([]byte{}, []byte{},
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", env.ProxyConfig{
			HttpProxy:  "http.example.com",
			HttpsProxy: "https.example.com",
			NoProxy:    "no_proxy.example.com",
		}, "virthost")

	envConfig = builder.defaultEnv()
	expected := `[Manager]
DefaultEnvironment=HTTP_PROXY="http.example.com"
DefaultEnvironment=HTTPS_PROXY="https.example.com"
DefaultEnvironment=NO_PROXY="no_proxy.example.com"
`
	if string(envConfig) != expected {
		t.Errorf("Invalid default env file with all vars set:\n%s", envConfig)
	}

	builder, _ = New([]byte{}, []byte{},
		"http://ironic.example.com",
		"quay.io/openshift-release-dev/ironic-ipa-image",
		"", "", "", env.ProxyConfig{
			HttpsProxy: "https.example.com",
		}, "virthost")

	envConfig = builder.defaultEnv()
	expected = `[Manager]
DefaultEnvironment=HTTPS_PROXY="https.example.com"
`
	if string(envConfig) != expected {
		t.Errorf("Invalid default env file with one var set:\n%s", envConfig)
	}
}
