package ignition

import (
	"reflect"
	"testing"

	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
)

func TestNMStateOutputToFiles(t *testing.T) {
	expectedMode := 0600
	expectedOverwrite := true
	tests := []struct {
		name            string
		generatedConfig []byte
		want            []ignition_config_types_32.File
		wantErr         bool
	}{
		{
			name:            "empty",
			generatedConfig: []byte(`---`),
			want:            []ignition_config_types_32.File{},
		},
		{
			name: "basic working",
			generatedConfig: []byte(`---
NetworkManager:
- - eth1.nmconnection
  - '[connection]

	id=eth1

	uuid=b6a3fd9a-4c76-4213-a698-8c0f0749b193

	type=ethernet

	interface-name=eth1

	permissions=


	[ethernet]

	mac-address-blacklist=


	[ipv4]

	dhcp-client-id=mac

	dns-search=

	method=disabled


	[ipv6]

	addr-gen-mode=eui64

	dhcp-duid=ll

	dhcp-iaid=mac

	dns-search=

	method=disabled


	[proxy]

	'
- - linux-br0.nmconnection
  - '[connection]

	id=linux-br0

	uuid=f942ffa5-668d-41f3-86bd-ef53e35565f4

	type=bridge

	autoconnect-slaves=1

	interface-name=linux-br0

	permissions=


	[bridge]


	[ipv4]

	dhcp-client-id=mac

	dns-search=

	method=disabled


	[ipv6]

	addr-gen-mode=eui64

	dhcp-duid=ll

	dhcp-iaid=mac

	dns-search=

	method=disabled


	[proxy]

	'
`),
			want: []ignition_config_types_32.File{
				{
					Node: ignition_config_types_32.Node{Path: "/etc/NetworkManager/system-connections/eth1.nmconnection", Overwrite: &expectedOverwrite},
					FileEmbedded1: ignition_config_types_32.FileEmbedded1{
						Contents: ignition_config_types_32.Resource{
							Source: ptr.To("data:text/plain,%5Bconnection%5D%0Aid%3Deth1%0Auuid%3Db6a3fd9a-4c76-4213-a698-8c0f0749b193%0Atype%3Dethernet%0Ainterface-name%3Deth1%0Apermissions%3D%0A%0A%5Bethernet%5D%0Amac-address-blacklist%3D%0A%0A%5Bipv4%5D%0Adhcp-client-id%3Dmac%0Adns-search%3D%0Amethod%3Ddisabled%0A%0A%5Bipv6%5D%0Aaddr-gen-mode%3Deui64%0Adhcp-duid%3Dll%0Adhcp-iaid%3Dmac%0Adns-search%3D%0Amethod%3Ddisabled%0A%0A%5Bproxy%5D%0A")},
						Mode: &expectedMode,
					},
				},
				{
					Node: ignition_config_types_32.Node{Path: "/etc/NetworkManager/system-connections/linux-br0.nmconnection", Overwrite: &expectedOverwrite},
					FileEmbedded1: ignition_config_types_32.FileEmbedded1{
						Contents: ignition_config_types_32.Resource{
							Source: ptr.To("data:text/plain,%5Bconnection%5D%0Aid%3Dlinux-br0%0Auuid%3Df942ffa5-668d-41f3-86bd-ef53e35565f4%0Atype%3Dbridge%0Aautoconnect-slaves%3D1%0Ainterface-name%3Dlinux-br0%0Apermissions%3D%0A%0A%5Bbridge%5D%0A%0A%5Bipv4%5D%0Adhcp-client-id%3Dmac%0Adns-search%3D%0Amethod%3Ddisabled%0A%0A%5Bipv6%5D%0Aaddr-gen-mode%3Deui64%0Adhcp-duid%3Dll%0Adhcp-iaid%3Dmac%0Adns-search%3D%0Amethod%3Ddisabled%0A%0A%5Bproxy%5D%0A")},
						Mode: &expectedMode,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nmstateOutputToFiles(tt.generatedConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("ignitionBuilder.nmstateOutputToFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Error(cmp.Diff(tt.want, got))
			}
		})
	}
}
