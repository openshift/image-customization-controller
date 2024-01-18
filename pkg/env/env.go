package env

import (
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
)

type EnvInputs struct {
	DeployISO                 string `envconfig:"DEPLOY_ISO" required:"true"`
	DeployInitrd              string `envconfig:"DEPLOY_INITRD" required:"true"`
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
