package env

import (
	"strings"
	"testing"
	"testing/fstest"
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

func TestPullSecretNotLoadedFromEnv(t *testing.T) {
	t.Setenv("DEPLOY_ISO", "iso")
	t.Setenv("DEPLOY_INITRD", "initrd")
	t.Setenv("IRONIC_AGENT_IMAGE", "image")
	t.Setenv("IRONIC_AGENT_PULL_SECRET", "should-be-ignored")

	inputs, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if inputs.IronicAgentPullSecret != "" {
		t.Fatalf("IRONIC_AGENT_PULL_SECRET must not be accepted from the environment, got %q", inputs.IronicAgentPullSecret)
	}
}

func TestLoadIronicAgentPullSecretFromFile(t *testing.T) {
	inputs := &EnvInputs{}
	fsys := fstest.MapFS{
		"run/secrets/pull-secret": {Data: []byte("from-file")},
	}

	if err := inputs.LoadIronicAgentPullSecret(fsys); err != nil {
		t.Fatal(err)
	}
	if inputs.IronicAgentPullSecret != "from-file" {
		t.Fatalf("expected pull secret from mounted file, got %q", inputs.IronicAgentPullSecret)
	}
}

func TestLoadIronicAgentPullSecretRequired(t *testing.T) {
	err := (&EnvInputs{}).LoadIronicAgentPullSecret(fstest.MapFS{})
	if err == nil {
		t.Fatal("expected error when pull secret file is missing")
	}
	if !strings.Contains(err.Error(), IronicAgentPullSecretPath) {
		t.Fatalf("expected error to mention %s, got %v", IronicAgentPullSecretPath, err)
	}
}
