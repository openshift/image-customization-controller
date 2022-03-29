package ignition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	vpath "github.com/coreos/vcontext/path"

	"github.com/openshift/image-customization-controller/pkg/env"
)

const (
	// https://github.com/openshift/ironic-image/blob/master/scripts/configure-coreos-ipa#L14
	ironicAgentPodmanFlags = "--tls-verify=false"

	// https://github.com/openshift/ironic-image/blob/master/scripts/configure-coreos-ipa#L11
	ironicInspectorVlanInterfaces = "all"
)

type ignitionBuilder struct {
	nmStateData           []byte
	registriesConf        []byte
	ironicBaseURL         string
	ironicAgentImage      string
	ironicAgentPullSecret string
	ironicRAMDiskSSHKey   string
	networkKeyFiles       []byte
	ipOptions             string
	proxy                 env.ProxyConfig
	hostname              string
}

func New(nmStateData, registriesConf []byte, ironicBaseURL, ironicAgentImage, ironicAgentPullSecret, ironicRAMDiskSSHKey, ipOptions string, proxy env.ProxyConfig, hostname string) (*ignitionBuilder, error) {
	if ironicBaseURL == "" {
		return nil, errors.New("ironicBaseURL is required")
	}
	if ironicAgentImage == "" {
		return nil, errors.New("ironicAgentImage is required")
	}

	return &ignitionBuilder{
		nmStateData:           nmStateData,
		registriesConf:        registriesConf,
		ironicBaseURL:         ironicBaseURL,
		ironicAgentImage:      ironicAgentImage,
		ironicAgentPullSecret: ironicAgentPullSecret,
		ironicRAMDiskSSHKey:   ironicRAMDiskSSHKey,
		ipOptions:             ipOptions,
		proxy:                 proxy,
		hostname:              hostname,
	}, nil
}

func (b *ignitionBuilder) ProcessNetworkState() (error, string) {
	if len(b.nmStateData) > 0 {
		nmstatectl := exec.Command("nmstatectl", "gc", "-")
		nmstatectl.Stdin = strings.NewReader(string(b.nmStateData))
		out, err := nmstatectl.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return err, string(ee.Stderr)
			}
			return err, ""
		}
		b.networkKeyFiles = out
	}
	return nil, ""
}

func (b *ignitionBuilder) Generate() ([]byte, error) {
	netFiles := []ignition_config_types_32.File{}
	if len(b.nmStateData) > 0 {
		nmstatectl := exec.Command("nmstatectl", "gc", "-")
		nmstatectl.Stdin = strings.NewReader(string(b.nmStateData))
		out, err := nmstatectl.Output()
		if err != nil {
			return nil, err
		}

		netFiles, err = nmstateOutputToFiles(out)
		if err != nil {
			return nil, err
		}
	}

	config := ignition_config_types_32.Config{
		Ignition: ignition_config_types_32.Ignition{
			Version: "3.2.0",
		},
		Storage: ignition_config_types_32.Storage{
			Files: []ignition_config_types_32.File{b.ironicPythonAgentConf()},
		},
		Systemd: ignition_config_types_32.Systemd{
			Units: []ignition_config_types_32.Unit{b.ironicAgentService(len(netFiles) > 0)},
		},
	}
	config.Storage.Files = append(config.Storage.Files, netFiles...)

	if b.ironicAgentPullSecret != "" {
		config.Storage.Files = append(config.Storage.Files, b.authFile())
	}

	if b.ironicRAMDiskSSHKey != "" {
		config.Passwd.Users = append(config.Passwd.Users, ignition_config_types_32.PasswdUser{
			Name: "core",
			SSHAuthorizedKeys: []ignition_config_types_32.SSHAuthorizedKey{
				ignition_config_types_32.SSHAuthorizedKey(strings.TrimSpace(b.ironicRAMDiskSSHKey)),
			},
		})
	}

	config.Storage.Files = append(config.Storage.Files, ignitionFileEmbed(
		"/etc/systemd/system.conf.d/10-default-env.conf",
		0644, false,
		b.defaultEnv()))

	config.Storage.Files = append(config.Storage.Files, ignitionFileEmbed(
		"/etc/NetworkManager/conf.d/clientid.conf",
		0644, false,
		[]byte("[connection]\nipv6.dhcp-duid=ll\nipv6.dhcp-iaid=mac")))

	update_hostname := fmt.Sprintf(`
	[[ "$DHCP6_FQDN_FQDN" =~ "." ]] && hostnamectl set-hostname --static --transient $DHCP6_FQDN_FQDN 
	[[ "$(< /proc/sys/kernel/hostname)" =~ (localhost|localhost.localdomain) ]] && hostnamectl set-hostname --transient %s`, b.hostname)

	config.Storage.Files = append(config.Storage.Files, ignitionFileEmbed(
		"/etc/NetworkManager/dispatcher.d/01-hostname",
		0744, false,
		[]byte(update_hostname)))

	if len(b.registriesConf) > 0 {
		registriesFile := ignitionFileEmbed("/etc/containers/registries.conf",
			0644, true,
			b.registriesConf)

		config.Storage.Files = append(config.Storage.Files, registriesFile)
	}

	report := config.Storage.Validate(vpath.ContextPath{})
	if report.IsFatal() {
		return nil, errors.New(report.String())
	}

	return json.Marshal(config)
}

func (b *ignitionBuilder) defaultEnv() []byte {
	buf := bytes.NewBufferString("[Manager]\n")

	setEnv := func(envVar, value string) {
		if value != "" {
			buf.WriteString(fmt.Sprintf("DefaultEnvironment=%s=\"%s\"\n",
				envVar, value))
		}
	}

	setEnv("HTTP_PROXY", b.proxy.HttpProxy)
	setEnv("HTTPS_PROXY", b.proxy.HttpsProxy)
	setEnv("NO_PROXY", b.proxy.NoProxy)
	return buf.Bytes()
}
