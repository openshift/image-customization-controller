/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package imageprovider

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/openshift/image-customization-controller/pkg/env"
)

func TestStreamArchSpecificURL(t *testing.T) {
	hostArch := env.HostArchitecture()
	nonHostArch := "aarch64"
	if hostArch == "aarch64" {
		nonHostArch = "x86_64"
	}

	const base = "http://host:8080/images/ironic-python-agent.rootfs"

	tests := []struct {
		name     string
		base     string
		stream   string
		arch     string
		expected string
	}{
		{
			name:     "host arch, no stream",
			base:     base,
			stream:   "",
			arch:     hostArch,
			expected: "http://host:8080/images/ironic-python-agent.rootfs",
		},
		{
			name:     "empty arch treated as host",
			base:     base,
			stream:   "",
			arch:     "",
			expected: "http://host:8080/images/ironic-python-agent.rootfs",
		},
		{
			name:     "non-host arch, no stream",
			base:     base,
			stream:   "",
			arch:     nonHostArch,
			expected: "http://host:8080/images/ironic-python-agent_" + nonHostArch + ".rootfs",
		},
		{
			name:     "host arch with stream",
			base:     base,
			stream:   "rhel-10",
			arch:     hostArch,
			expected: "http://host:8080/images/ironic-python-agent-rhel-10.rootfs",
		},
		{
			name:     "non-host arch with stream",
			base:     base,
			stream:   "rhel-10",
			arch:     nonHostArch,
			expected: "http://host:8080/images/ironic-python-agent-rhel-10_" + nonHostArch + ".rootfs",
		},
		{
			name:     "empty arch with stream treated as host",
			base:     base,
			stream:   "rhel-10",
			arch:     "",
			expected: "http://host:8080/images/ironic-python-agent-rhel-10.rootfs",
		},
		{
			name:     "no extension",
			base:     "http://host:8080/images/rootfs",
			stream:   "rhel-10",
			arch:     nonHostArch,
			expected: "http://host:8080/images/rootfs-rhel-10_" + nonHostArch,
		},
		{
			name:     "invalid URL returned as-is",
			base:     "://not-a-url",
			stream:   "rhel-10",
			arch:     nonHostArch,
			expected: "://not-a-url",
		},
		{
			name:     "path only base",
			base:     "http://host/agent.img",
			stream:   "rhel-9",
			arch:     hostArch,
			expected: "http://host/agent-rhel-9.img",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := streamArchSpecificURL(tc.base, tc.stream, tc.arch)
			if result != tc.expected {
				t.Errorf("streamArchSpecificURL(%q, %q, %q) = %q, want %q",
					tc.base, tc.stream, tc.arch, result, tc.expected)
			}
		})
	}
}

func TestStreamFromImageData(t *testing.T) {
	tests := []struct {
		name     string
		data     imageprovider.ImageData
		expected string
	}{
		{
			name:     "nil metadata",
			data:     imageprovider.ImageData{},
			expected: "",
		},
		{
			name: "nil labels",
			data: imageprovider.ImageData{
				ImageMetadata: &metav1.ObjectMeta{},
			},
			expected: "",
		},
		{
			name: "labels without stream key",
			data: imageprovider.ImageData{
				ImageMetadata: &metav1.ObjectMeta{
					Labels: map[string]string{
						"other-label": "value",
					},
				},
			},
			expected: "",
		},
		{
			name: "stream label present",
			data: imageprovider.ImageData{
				ImageMetadata: &metav1.ObjectMeta{
					Labels: map[string]string{
						"coreos.openshift.io/stream": "rhel-10",
					},
				},
			},
			expected: "rhel-10",
		},
		{
			name: "empty stream label",
			data: imageprovider.ImageData{
				ImageMetadata: &metav1.ObjectMeta{
					Labels: map[string]string{
						"coreos.openshift.io/stream": "",
					},
				},
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := streamFromImageData(tc.data)
			if result != tc.expected {
				t.Errorf("streamFromImageData() = %q, want %q", result, tc.expected)
			}
		})
	}
}
