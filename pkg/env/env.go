package env

import (
	"os"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
)

type EnvInputs struct {
	DeployISO                 string `envconfig:"DEPLOY_ISO" required:"true"`
	DeployInitrd              string `envconfig:"DEPLOY_INITRD" required:"true"`
	ImageSharedDir            string `envconfig:"IMAGE_SHARED_DIR"`
	IronicBaseURL             string `envconfig:"IRONIC_BASE_URL"`
	IronicInspectorBaseURL    string `envconfig:"IRONIC_INSPECTOR_BASE_URL"`
	IronicAgentImage          string `envconfig:"IRONIC_AGENT_IMAGE" required:"true"`
	IronicAgentPullSecret     string `envconfig:"IRONIC_AGENT_PULL_SECRET"`
	IronicAgentVlanInterfaces string `envconfig:"IRONIC_AGENT_VLAN_INTERFACES"`
	IronicRAMDiskSSHKey       string `envconfig:"IRONIC_RAMDISK_SSH_KEY"`
	RegistriesConfPath        string `envconfig:"REGISTRIES_CONF_PATH"`
	IpOptions                 string `envconfig:"IP_OPTIONS"`
	HttpProxy                 string `envconfig:"HTTP_PROXY"`
	HttpsProxy                string `envconfig:"HTTPS_PROXY"`
	NoProxy                   string `envconfig:"NO_PROXY"`
	AdditionalNTPServers      string `envconfig:"ADDITIONAL_NTP_SERVERS"`
	CaBundle                  string `envconfig:"CA_BUNDLE"`
}

func New() (*EnvInputs, error) {
	env := &EnvInputs{}
	err := envconfig.Process("", env)
	return env, err
}

func (env *EnvInputs) RegistriesConf() (data []byte, err error) {
	if env.RegistriesConfPath == "" {
		return
	}

	data, err = os.ReadFile(env.RegistriesConfPath)
	if err != nil {
		err = errors.Wrapf(err, "failed to read registries.conf file %s",
			env.RegistriesConfPath)
	}
	return
}

// HostArchitecture returns the standardized architecture name for the current host.
// Maps Go's GOARCH values to the architecture names used in image files.
func HostArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}
