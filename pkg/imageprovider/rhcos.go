package imageprovider

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/go-logr/logr"

	metal3 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/openshift/image-customization-controller/pkg/env"
	"github.com/openshift/image-customization-controller/pkg/ignition"
	"github.com/openshift/image-customization-controller/pkg/imagehandler"
)

type rhcosImageProvider struct {
	ImageHandler   imagehandler.ImageHandler
	EnvInputs      *env.EnvInputs
	RegistriesConf []byte
}

func NewRHCOSImageProvider(imageServer imagehandler.ImageHandler, inputs *env.EnvInputs) imageprovider.ImageProvider {
	registries, err := inputs.RegistriesConf()
	if err != nil {
		panic(err)
	}

	return &rhcosImageProvider{
		ImageHandler:   imageServer,
		EnvInputs:      inputs,
		RegistriesConf: registries,
	}
}

func (ip *rhcosImageProvider) SupportsArchitecture(arch string) bool {
	return ip.ImageHandler.HasImagesForArchitecture(arch)
}

func (ip *rhcosImageProvider) SupportsFormat(format metal3.ImageFormat) bool {
	switch format {
	case metal3.ImageFormatISO, metal3.ImageFormatInitRD:
		return true
	default:
		return false
	}
}

func (ip *rhcosImageProvider) buildIgnitionConfig(networkData imageprovider.NetworkData, hostname string) ([]byte, error) {
	nmstateData := networkData["nmstate"]

	additionalNTPServers := []string{}
	if ip.EnvInputs.AdditionalNTPServers != "" {
		additionalNTPServers = strings.Split(ip.EnvInputs.AdditionalNTPServers, ",")
	}

	builder, err := ignition.New(nmstateData, ip.RegistriesConf,
		ip.EnvInputs.IronicBaseURL,
		ip.EnvInputs.IronicInspectorBaseURL,
		ip.EnvInputs.IronicAgentImage,
		ip.EnvInputs.IronicAgentPullSecret,
		ip.EnvInputs.IronicRAMDiskSSHKey,
		ip.EnvInputs.IpOptions,
		ip.EnvInputs.HttpProxy,
		ip.EnvInputs.HttpsProxy,
		ip.EnvInputs.NoProxy,
		hostname,
		ip.EnvInputs.IronicAgentVlanInterfaces,
		additionalNTPServers,
		ip.EnvInputs.CaBundle,
	)
	if err != nil {
		return nil, imageprovider.BuildInvalidError(err)
	}

	message, err := builder.ProcessNetworkState()
	if message != "" {
		return nil, imageprovider.BuildInvalidError(errors.New(message))
	}
	if err != nil {
		return nil, err
	}

	return builder.Generate()
}

func imageKey(data imageprovider.ImageData) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s.%s",
		data.ImageMetadata.Namespace,
		data.ImageMetadata.Name,
		data.ImageMetadata.UID,
		streamFromImageData(data),
		data.Architecture,
		data.Format,
	)
}

func streamFromImageData(data imageprovider.ImageData) string {
	if data.ImageMetadata != nil && data.ImageMetadata.Labels != nil {
		return data.ImageMetadata.Labels["coreos.openshift.io/stream"]
	}
	return ""
}

func (ip *rhcosImageProvider) BuildImage(data imageprovider.ImageData, networkData imageprovider.NetworkData, log logr.Logger) (imageprovider.GeneratedImage, error) {
	generated := imageprovider.GeneratedImage{}
	ignitionConfig, err := ip.buildIgnitionConfig(networkData, data.ImageMetadata.Name)
	if err != nil {
		return generated, err
	}

	stream := streamFromImageData(data)

	url, err := ip.ImageHandler.ServeImage(imageKey(data), data.Architecture, stream, ignitionConfig,
		data.Format == metal3.ImageFormatInitRD, false)
	if errors.As(err, &imagehandler.InvalidBaseImageError{}) {
		return generated, imageprovider.BuildInvalidError(err)
	}
	if err != nil {
		return generated, err
	}
	generated.ImageURL = url

	if data.Format == metal3.ImageFormatInitRD {
		kernelURL, err := ip.ImageHandler.ServeKernel(data.Architecture, stream)
		if err != nil {
			return generated, err
		}
		if kernelURL == "" && data.Architecture != env.HostArchitecture() {
			return generated, fmt.Errorf("no kernel file available for architecture %s", data.Architecture)
		}
		generated.KernelURL = kernelURL

		// Set the rootfs URL for every node. The stream and architecture
		// determine which rootfs image is used (e.g. rhel-9 vs rhel-10,
		// x86_64 vs aarch64).
		if ip.EnvInputs.IronicRootfsURL != "" {
			rootfsURL := streamArchSpecificURL(ip.EnvInputs.IronicRootfsURL, stream, data.Architecture)
			generated.ExtraKernelParams = "coreos.live.rootfs_url=" + rootfsURL
		}
	}

	return generated, nil
}

// streamArchSpecificURL transforms a base URL like
// "http://host:port/images/ironic-python-agent.rootfs" into a stream- and/or
// arch-specific URL. The stream suffix (e.g. "-rhel-10") is added when stream
// is non-empty, and the arch suffix (e.g. "_aarch64") is added when the
// architecture differs from the host. Examples:
//
//	stream="rhel-10", arch=host  → ironic-python-agent-rhel-10.rootfs
//	stream="",        arch=arm64 → ironic-python-agent_aarch64.rootfs
//	stream="rhel-10", arch=arm64 → ironic-python-agent-rhel-10_aarch64.rootfs
func streamArchSpecificURL(baseURL, stream, arch string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	if arch == "" {
		arch = env.HostArchitecture()
	}
	ext := path.Ext(u.Path)
	base := strings.TrimSuffix(u.Path, ext)
	if stream != "" {
		base = fmt.Sprintf("%s-%s", base, stream)
	}
	if arch != env.HostArchitecture() {
		base = fmt.Sprintf("%s_%s", base, arch)
	}
	u.Path = base + ext
	return u.String()
}

func (ip *rhcosImageProvider) DiscardImage(data imageprovider.ImageData) error {
	ip.ImageHandler.RemoveImage(imageKey(data))
	return nil
}
