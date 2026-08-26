package env

import (
	"io/fs"
	"os"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
)

const (
	// IronicAgentPullSecretPath is the in-container path of the pull secret
	// volume mount provided by cluster-baremetal-operator and the installer.
	IronicAgentPullSecretPath = "/run/secrets/pull-secret" // #nosec G101
	ironicAgentPullSecretFile = "run/secrets/pull-secret"  // #nosec G101
)

type EnvInputs struct {
	DeployISO              string `envconfig:"DEPLOY_ISO" required:"true"`
	DeployInitrd           string `envconfig:"DEPLOY_INITRD" required:"true"`
	DeployKernel           string `envconfig:"DEPLOY_KERNEL"`
	ImageSharedDir         string `envconfig:"IMAGE_SHARED_DIR"`
	IronicBaseURL          string `envconfig:"IRONIC_BASE_URL"`
	IronicInspectorBaseURL string `envconfig:"IRONIC_INSPECTOR_BASE_URL"`
	IronicAgentImage       string `envconfig:"IRONIC_AGENT_IMAGE" required:"true"`
	// IronicAgentPullSecret is filled from the mounted file, not from the environment.
	IronicAgentPullSecret     string
	IronicAgentVlanInterfaces string `envconfig:"IRONIC_AGENT_VLAN_INTERFACES"`
	IronicRAMDiskSSHKey       string `envconfig:"IRONIC_RAMDISK_SSH_KEY"`
	RegistriesConfPath        string `envconfig:"REGISTRIES_CONF_PATH"`
	IpOptions                 string `envconfig:"IP_OPTIONS"`
	HttpProxy                 string `envconfig:"HTTP_PROXY"`
	HttpsProxy                string `envconfig:"HTTPS_PROXY"`
	NoProxy                   string `envconfig:"NO_PROXY"`
	AdditionalNTPServers      string `envconfig:"ADDITIONAL_NTP_SERVERS"`
	CaBundle                  string `envconfig:"CA_BUNDLE"`
	IronicRootfsURL           string `envconfig:"IRONIC_ROOTFS_URL"`
}

// LoadIronicAgentPullSecret reads the Ironic agent pull secret from the mounted file.
func (env *EnvInputs) LoadIronicAgentPullSecret(fsys fs.FS) error {
	data, err := fs.ReadFile(fsys, ironicAgentPullSecretFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read pull secret from %s", IronicAgentPullSecretPath)
	}
	env.IronicAgentPullSecret = string(data)
	return nil
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
