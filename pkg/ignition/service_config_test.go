package ignition

import (
	"reflect"
	"testing"

	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/pointer"
)

func TestIronicPythonAgentConf(t *testing.T) {
	expectedMode := 0644
	expectedOverwrite := false
	tests := []struct {
		name                          string
		ironicBaseURL                 string
		ironicInspectorVlanInterfaces string
		want                          ignition_config_types_32.File
	}{
		{
			name:          "basic",
			ironicBaseURL: "http://example.com/foo",
			want: ignition_config_types_32.File{
				Node: ignition_config_types_32.Node{Path: "/etc/ironic-python-agent.conf", Overwrite: &expectedOverwrite},
				FileEmbedded1: ignition_config_types_32.FileEmbedded1{
					Contents: ignition_config_types_32.Resource{
						Source: pointer.StringPtr("data:text/plain,%0A%5BDEFAULT%5D%0Aapi_url%20%3D%20http%3A%2F%2Fexample.com%2Ffoo%3A6385%0Ainspection_callback_url%20%3D%20http%3A%2F%2Fexample.com%2Ffoo%3A5050%2Fv1%2Fcontinue%0Ainsecure%20%3D%20True%0A%0Acollect_lldp%20%3D%20True%0Aenable_vlan_interfaces%20%3D%20all%0Ainspection_collectors%20%3D%20default%2Cextra-hardware%2Clogs%0Ainspection_dhcp_all_interfaces%20%3D%20True%0A")},
					Mode: &expectedMode},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ignitionBuilder{
				ironicBaseURL: tt.ironicBaseURL,
			}
			if got := b.ironicPythonAgentConf(); !reflect.DeepEqual(got, tt.want) {
				t.Error(cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestIronicAgentService(t *testing.T) {
	tests := []struct {
		name                  string
		ironicAgentImage      string
		ironicAgentPullSecret string
		want                  ignition_config_types_32.Unit
	}{
		{
			name:                  "basic",
			ironicAgentImage:      "http://example.com/foo:latest",
			ironicAgentPullSecret: "foo",
			want: ignition_config_types_32.Unit{
				Name:     "ironic-agent.service",
				Enabled:  pointer.BoolPtr(true),
				Contents: pointer.StringPtr("[Unit]\nDescription=Ironic Agent\nAfter=network-online.target\nWants=network-online.target\n[Service]\nEnvironment=\"HTTP_PROXY=\"\nEnvironment=\"HTTPS_PROXY=\"\nEnvironment=\"NO_PROXY=\"\nTimeoutStartSec=0\nRestart=on-failure\nExecStartPre=/bin/podman pull http://example.com/foo:latest --tls-verify=false --authfile=/etc/authfile.json\nExecStart=/bin/podman run --privileged --network host --mount type=bind,src=/etc/ironic-python-agent.conf,dst=/etc/ironic-python-agent/ignition.conf --mount type=bind,src=/dev,dst=/dev --mount type=bind,src=/sys,dst=/sys --mount type=bind,src=/run/dbus/system_bus_socket,dst=/run/dbus/system_bus_socket --mount type=bind,src=/,dst=/mnt/coreos --env \"IPA_COREOS_IP_OPTIONS=ip=dhcp6\" --name ironic-agent http://example.com/foo:latest\n[Install]\nWantedBy=multi-user.target\n"),
			},
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ignitionBuilder{
				ironicAgentImage:      tt.ironicAgentImage,
				ironicAgentPullSecret: tt.ironicAgentPullSecret,
				ipOptions:             "ip=dhcp6",
			}
			if got := b.ironicAgentService(); !reflect.DeepEqual(got, tt.want) {
				t.Error(cmp.Diff(tt.want, got))
			}
		})
	}
}
