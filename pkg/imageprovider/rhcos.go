package imageprovider

import (
	"errors"
	"fmt"
	"path/filepath"
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
	nmstateData, found := networkData["nmstate"]
	if len(networkData) > 0 && !found {
		return nil, imageprovider.BuildInvalidError(
			fmt.Errorf("network data Secret provided but missing required 'nmstate' key"),
		)
	}

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
	return fmt.Sprintf("%s-%s-%s-%s.%s",
		data.ImageMetadata.Namespace,
		data.ImageMetadata.Name,
		data.ImageMetadata.UID,
		data.Architecture,
		data.Format,
	)
}

func (ip *rhcosImageProvider) BuildImage(data imageprovider.ImageData, networkData imageprovider.NetworkData, log logr.Logger) (imageprovider.GeneratedImage, error) {
	generated := imageprovider.GeneratedImage{}
	ignitionConfig, err := ip.buildIgnitionConfig(networkData, data.ImageMetadata.Name)
	if err != nil {
		return generated, err
	}

	url, err := ip.ImageHandler.ServeImage(imageKey(data), data.Architecture, ignitionConfig,
		data.Format == metal3.ImageFormatInitRD, false)
	if errors.As(err, &imagehandler.InvalidBaseImageError{}) {
		return generated, imageprovider.BuildInvalidError(err)
	}
	if err != nil {
		return generated, err
	}
	generated.ImageURL = url

	if data.Format == metal3.ImageFormatInitRD {
		kernelURL, err := ip.ImageHandler.ServeKernel(data.Architecture)
		if err != nil {
			return generated, err
		}
		generated.KernelURL = kernelURL

		// Override the rootfs URL for non-host architectures. Ironic's global
		// kernel_append_params contains a rootfs URL for the host architecture.
		// For other architectures we need to point to the arch-specific rootfs.
		if ip.EnvInputs.IronicRootfsURL != "" && data.Architecture != env.HostArchitecture() {
			archRootfsURL := archSpecificURL(ip.EnvInputs.IronicRootfsURL, data.Architecture)
			generated.ExtraKernelParams = "coreos.live.rootfs_url=" + archRootfsURL
		}
	}

	return generated, nil
}

// archSpecificURL transforms a base URL like
// "http://host:port/images/ironic-python-agent.rootfs" into an arch-specific
// URL like "http://host:port/images/ironic-python-agent_aarch64.rootfs".
func archSpecificURL(baseURL, arch string) string {
	ext := filepath.Ext(baseURL)
	base := strings.TrimSuffix(baseURL, ext)
	return fmt.Sprintf("%s_%s%s", base, arch, ext)
}

func (ip *rhcosImageProvider) DiscardImage(data imageprovider.ImageData) error {
	ip.ImageHandler.RemoveImage(imageKey(data))
	return nil
}
