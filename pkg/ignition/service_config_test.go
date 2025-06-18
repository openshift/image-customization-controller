package ignition

import (
	"reflect"
	"testing"

	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
)

func TestIronicPythonAgentConf(t *testing.T) {
	expectedMode := 0644
	expectedOverwrite := false
	tests := []struct {
		name                          string
		ironicBaseURL                 string
		ironicInspectorBaseURL        string
		ironicInspectorVlanInterfaces string
		want                          ignition_config_types_32.File
	}{
		{
			name:                          "basic",
			ironicBaseURL:                 "http://example.com/foo",
			ironicInspectorBaseURL:        "http://example.com/bar",
			ironicInspectorVlanInterfaces: "all",
			want: ignition_config_types_32.File{
				Node: ignition_config_types_32.Node{Path: "/etc/ironic-python-agent.conf", Overwrite: &expectedOverwrite},
				FileEmbedded1: ignition_config_types_32.FileEmbedded1{
					Contents: ignition_config_types_32.Resource{
						Source: pointer.String("data:text/plain,%0A%5BDEFAULT%5D%0Aapi_url%20%3D%20http%3A%2F%2Fexample.com%3A6385%2Ffoo%0Ainspection_callback_url%20%3D%20http%3A%2F%2Fexample.com%3A5050%2Fbar%2Fv1%2Fcontinue%0Ainsecure%20%3D%20True%0Aenable_vlan_interfaces%20%3D%20all%0A")},
					Mode: &expectedMode},
			},
		},
		{
			name:                          "basic_no_vlans",
			ironicBaseURL:                 "http://example.com/foo",
			ironicInspectorBaseURL:        "http://example.com/bar",
			ironicInspectorVlanInterfaces: "",
			want: ignition_config_types_32.File{
				Node: ignition_config_types_32.Node{Path: "/etc/ironic-python-agent.conf", Overwrite: &expectedOverwrite},
				FileEmbedded1: ignition_config_types_32.FileEmbedded1{
					Contents: ignition_config_types_32.Resource{
						Source: pointer.String("data:text/plain,%0A%5BDEFAULT%5D%0Aapi_url%20%3D%20http%3A%2F%2Fexample.com%3A6385%2Ffoo%0Ainspection_callback_url%20%3D%20http%3A%2F%2Fexample.com%3A5050%2Fbar%2Fv1%2Fcontinue%0Ainsecure%20%3D%20True%0Aenable_vlan_interfaces%20%3D%20%0A")},
					Mode: &expectedMode},
			},
		},
		{
			name:                          "new-style",
			ironicBaseURL:                 "http://192.0.2.1,https://[2001:db8::1]",
			ironicInspectorBaseURL:        "",
			ironicInspectorVlanInterfaces: "all",
			want: ignition_config_types_32.File{
				Node: ignition_config_types_32.Node{Path: "/etc/ironic-python-agent.conf", Overwrite: &expectedOverwrite},
				FileEmbedded1: ignition_config_types_32.FileEmbedded1{
					Contents: ignition_config_types_32.Resource{
						Source: pointer.String("data:text/plain,%0A%5BDEFAULT%5D%0Aapi_url%20%3D%20http%3A%2F%2F192.0.2.1%3A6385%2Chttps%3A%2F%2F%5B2001%3Adb8%3A%3A1%5D%3A6385%0Ainspection_callback_url%20%3D%20%0Ainsecure%20%3D%20True%0Aenable_vlan_interfaces%20%3D%20all%0A")},
					Mode: &expectedMode},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ignitionBuilder{
				ironicBaseURL:          tt.ironicBaseURL,
				ironicInspectorBaseURL: tt.ironicInspectorBaseURL,
			}
			if got := b.IronicAgentConf(tt.ironicInspectorVlanInterfaces); !reflect.DeepEqual(got, tt.want) {
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
		copyNetwork           bool
		want                  ignition_config_types_32.Unit
	}{
		{
			name:                  "basic",
			ironicAgentImage:      "http://example.com/foo:latest",
			ironicAgentPullSecret: "foo",
			want: ignition_config_types_32.Unit{
				Name:     "ironic-agent.service",
				Enabled:  pointer.Bool(true),
				Contents: pointer.String("[Unit]\nDescription=Ironic Agent\nAfter=network-online.target\nWants=network-online.target\n[Service]\nEnvironment=\"HTTP_PROXY=\"\nEnvironment=\"HTTPS_PROXY=\"\nEnvironment=\"NO_PROXY=\"\nTimeoutStartSec=0\nRestart=on-failure\nRestartSec=5\nStartLimitIntervalSec=0\nType=notify\nExecStartPre=/bin/rm -f %t/%n.ctr-id\nExecStart=/bin/podman run --detach --cgroups=no-conmon --sdnotify=conmon --rm --cidfile=%t/%n.ctr-id --privileged --network host --mount type=bind,src=/etc/ironic-python-agent.conf,dst=/etc/ironic-python-agent/ignition.conf --mount type=bind,src=/dev,dst=/dev --mount type=bind,src=/sys,dst=/sys --mount type=bind,src=/run/dbus/system_bus_socket,dst=/run/dbus/system_bus_socket --mount type=bind,src=/,dst=/mnt/coreos --mount type=bind,src=/run/udev,dst=/run/udev --ipc=host --uts=host --env \"IPA_COREOS_IP_OPTIONS=ip=dhcp6\" --env IPA_COREOS_COPY_NETWORK=false --env \"IPA_DEFAULT_HOSTNAME=my-host\"  --authfile=/etc/authfile.json --name ironic-agent http://example.com/foo:latest\nExecStop=/usr/bin/podman stop --ignore --cidfile=%t/%n.ctr-id\nExecStopPost=/usr/bin/podman rm -f --ignore --cidfile=%t/%n.ctr-id\n[Install]\nWantedBy=multi-user.target\n"),
			},
		},
		{
			name:             "no pull secret",
			ironicAgentImage: "http://example.com/foo:latest",
			want: ignition_config_types_32.Unit{
				Name:     "ironic-agent.service",
				Enabled:  pointer.Bool(true),
				Contents: pointer.String("[Unit]\nDescription=Ironic Agent\nAfter=network-online.target\nWants=network-online.target\n[Service]\nEnvironment=\"HTTP_PROXY=\"\nEnvironment=\"HTTPS_PROXY=\"\nEnvironment=\"NO_PROXY=\"\nTimeoutStartSec=0\nRestart=on-failure\nRestartSec=5\nStartLimitIntervalSec=0\nType=notify\nExecStartPre=/bin/rm -f %t/%n.ctr-id\nExecStart=/bin/podman run --detach --cgroups=no-conmon --sdnotify=conmon --rm --cidfile=%t/%n.ctr-id --privileged --network host --mount type=bind,src=/etc/ironic-python-agent.conf,dst=/etc/ironic-python-agent/ignition.conf --mount type=bind,src=/dev,dst=/dev --mount type=bind,src=/sys,dst=/sys --mount type=bind,src=/run/dbus/system_bus_socket,dst=/run/dbus/system_bus_socket --mount type=bind,src=/,dst=/mnt/coreos --mount type=bind,src=/run/udev,dst=/run/udev --ipc=host --uts=host --env \"IPA_COREOS_IP_OPTIONS=ip=dhcp6\" --env IPA_COREOS_COPY_NETWORK=false --env \"IPA_DEFAULT_HOSTNAME=my-host\"  --name ironic-agent http://example.com/foo:latest\nExecStop=/usr/bin/podman stop --ignore --cidfile=%t/%n.ctr-id\nExecStopPost=/usr/bin/podman rm -f --ignore --cidfile=%t/%n.ctr-id\n[Install]\nWantedBy=multi-user.target\n"),
			},
		},
		{
			name:                  "copy network",
			ironicAgentImage:      "http://example.com/foo:latest",
			ironicAgentPullSecret: "foo",
			copyNetwork:           true,
			want: ignition_config_types_32.Unit{
				Name:     "ironic-agent.service",
				Enabled:  pointer.Bool(true),
				Contents: pointer.String("[Unit]\nDescription=Ironic Agent\nAfter=network-online.target\nWants=network-online.target\n[Service]\nEnvironment=\"HTTP_PROXY=\"\nEnvironment=\"HTTPS_PROXY=\"\nEnvironment=\"NO_PROXY=\"\nTimeoutStartSec=0\nRestart=on-failure\nRestartSec=5\nStartLimitIntervalSec=0\nType=notify\nExecStartPre=/bin/rm -f %t/%n.ctr-id\nExecStart=/bin/podman run --detach --cgroups=no-conmon --sdnotify=conmon --rm --cidfile=%t/%n.ctr-id --privileged --network host --mount type=bind,src=/etc/ironic-python-agent.conf,dst=/etc/ironic-python-agent/ignition.conf --mount type=bind,src=/dev,dst=/dev --mount type=bind,src=/sys,dst=/sys --mount type=bind,src=/run/dbus/system_bus_socket,dst=/run/dbus/system_bus_socket --mount type=bind,src=/,dst=/mnt/coreos --mount type=bind,src=/run/udev,dst=/run/udev --ipc=host --uts=host --env \"IPA_COREOS_IP_OPTIONS=ip=dhcp6\" --env IPA_COREOS_COPY_NETWORK=true --env \"IPA_DEFAULT_HOSTNAME=my-host\"  --authfile=/etc/authfile.json --name ironic-agent http://example.com/foo:latest\nExecStop=/usr/bin/podman stop --ignore --cidfile=%t/%n.ctr-id\nExecStopPost=/usr/bin/podman rm -f --ignore --cidfile=%t/%n.ctr-id\n[Install]\nWantedBy=multi-user.target\n"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &ignitionBuilder{
				ironicAgentImage:      tt.ironicAgentImage,
				ironicAgentPullSecret: tt.ironicAgentPullSecret,
				ipOptions:             "ip=dhcp6",
				hostname:              "my-host",
			}
			if got := b.IronicAgentService(tt.copyNetwork); !reflect.DeepEqual(got, tt.want) {
				t.Error(cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestProcessURLs(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultPath string
		want        string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "single",
			input: "http://192.0.2.1",
			want:  "http://192.0.2.1:6385",
		},
		{
			name:  "single-with-port",
			input: "http://192.0.2.1:42",
			want:  "http://192.0.2.1:42",
		},
		{
			name:  "single-v6-tls",
			input: "https://[2001:db8::1]",
			want:  "https://[2001:db8::1]:6385",
		},
		{
			name:  "single-with-port",
			input: "https://[2001:db8::1]:42",
			want:  "https://[2001:db8::1]:42",
		},
		{
			name:  "dual",
			input: "http://192.0.2.1,https://[2001:db8::1]",
			want:  "http://192.0.2.1:6385,https://[2001:db8::1]:6385",
		},
		{
			name:  "dual-with-port",
			input: "http://192.0.2.1:42,https://[2001:db8::1]:43",
			want:  "http://192.0.2.1:42,https://[2001:db8::1]:43",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := processURLs(tt.input, tt.defaultPath, "6385")
			assert.Equal(t, tt.want, got)
		})
	}
}
