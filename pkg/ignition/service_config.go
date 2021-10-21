package ignition

import (
	"fmt"
	"strings"

	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	"k8s.io/utils/pointer"
)

func (b *ignitionBuilder) ironicPythonAgentConf() ignition_config_types_32.File {
	template := `
[DEFAULT]
api_url = %s:6385
inspection_callback_url = %s:5050/v1/continue
insecure = True

collect_lldp = True
enable_vlan_interfaces = %s
inspection_collectors = default,extra-hardware,logs
inspection_dhcp_all_interfaces = True
`
	contents := fmt.Sprintf(template, b.ironicBaseURL, b.ironicBaseURL, ironicInspectorVlanInterfaces)
	return ignitionFileEmbed("/etc/ironic-python-agent.conf", []byte(contents))
}

func (b *ignitionBuilder) ironicAgentService() ignition_config_types_32.Unit {
	flags := ironicAgentPodmanFlags
	if b.ironicAgentPullSecret != "" {
		flags += " --authfile=/etc/authfile.json"
	}

	unitTemplate := `[Unit]
Description=Ironic Agent
After=network-online.target
Wants=network-online.target
[Service]
TimeoutStartSec=0
ExecStartPre=/bin/podman pull %s %s
ExecStart=/bin/podman run --privileged --network host --mount type=bind,src=/etc/ironic-python-agent.conf,dst=/etc/ironic-python-agent/ignition.conf --mount type=bind,src=/dev,dst=/dev --mount type=bind,src=/sys,dst=/sys --mount type=bind,src=/,dst=/mnt/coreos --name ironic-agent %s
[Install]
WantedBy=multi-user.target
`
	contents := fmt.Sprintf(unitTemplate, b.ironicAgentImage, flags, b.ironicAgentImage)

	return ignition_config_types_32.Unit{
		Name:     "ironic-agent.service",
		Enabled:  pointer.BoolPtr(true),
		Contents: &contents,
	}
}

func (b *ignitionBuilder) authFile() ignition_config_types_32.File {
	source := "data:;base64," + strings.TrimSpace(b.ironicAgentPullSecret)
	return ignition_config_types_32.File{
		Node:          ignition_config_types_32.Node{Path: "/etc/authfile.json"},
		FileEmbedded1: ignition_config_types_32.FileEmbedded1{Contents: ignition_config_types_32.Resource{Source: &source}},
	}
}