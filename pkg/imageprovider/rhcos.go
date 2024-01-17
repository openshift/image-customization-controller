package imageprovider

import (
	"errors"
	"fmt"

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
	return true
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
	)
	if err != nil {
		return nil, imageprovider.BuildInvalidError(err)
	}

	err, message := builder.ProcessNetworkState()
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

	url, err := ip.ImageHandler.ServeImage(imageKey(data), ignitionConfig,
		data.Format == metal3.ImageFormatInitRD, false)
	if errors.As(err, &imagehandler.InvalidBaseImageError{}) {
		return generated, imageprovider.BuildInvalidError(err)
	}
	generated.ImageURL = url
	return generated, err
}

func (ip *rhcosImageProvider) DiscardImage(data imageprovider.ImageData) error {
	ip.ImageHandler.RemoveImage(imageKey(data))
	return nil
}
