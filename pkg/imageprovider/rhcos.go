package imageprovider

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

const (
	infraEnvLabel              string = "infraenvs.agent-install.openshift.io"
	ignitionOverrideAnnotation string = "baremetal.openshift.io/ignition-override-uri"
)

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

func (ip *rhcosImageProvider) buildIgnitionConfig(networkData imageprovider.NetworkData, hostname string, mergeWith []byte) ([]byte, error) {
	nmstateData := networkData["nmstate"]

	builder, err := ignition.New(nmstateData, ip.RegistriesConf,
		ip.EnvInputs.IronicBaseURL,
		ip.EnvInputs.IronicAgentImage,
		ip.EnvInputs.IronicAgentPullSecret,
		ip.EnvInputs.IronicRAMDiskSSHKey,
		ip.EnvInputs.IpOptions,
		ip.EnvInputs.HttpProxy,
		ip.EnvInputs.HttpsProxy,
		ip.EnvInputs.NoProxy,
		hostname,
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

	return builder.GenerateAndMergeWith(mergeWith)
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

func getIgnitionOverride(imageMetadata *metav1.ObjectMeta, log logr.Logger) ([]byte, error) {
	if overrideURI, exist := imageMetadata.Annotations[ignitionOverrideAnnotation]; exist {
		log.Info("using Ignition override when building the image", "host", imageMetadata.Name, "overrideURI", overrideURI)
		resp, err := http.Get(overrideURI) //#nosec G107
		if err != nil {
			return nil, errors.Wrap(err, "could not download Ignition override")
		}
		defer resp.Body.Close()

		override, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrap(err, "could not download Ignition override")
		}

		return override, nil
	}

	if infraEnvName, useInfraEnv := imageMetadata.Labels[infraEnvLabel]; useInfraEnv {
		log.Info("host is using an InfraEnv, waiting for Ignition override", "host", imageMetadata.Name, "infraEnv", infraEnvName)
		return nil, imageprovider.ImageNotReady{}
	}

	return nil, nil
}

func (ip *rhcosImageProvider) BuildImage(data imageprovider.ImageData, networkData imageprovider.NetworkData, log logr.Logger) (string, error) {
	mergeWith, err := getIgnitionOverride(data.ImageMetadata, log)
	if err != nil {
		return "", err
	}

	ignitionConfig, err := ip.buildIgnitionConfig(networkData, data.ImageMetadata.Name, mergeWith)
	if err != nil {
		return "", err
	}

	url, err := ip.ImageHandler.ServeImage(imageKey(data), ignitionConfig,
		data.Format == metal3.ImageFormatInitRD, false)
	if errors.As(err, &imagehandler.InvalidBaseImageError{}) {
		return "", imageprovider.BuildInvalidError(err)
	}
	return url, err
}

func (ip *rhcosImageProvider) DiscardImage(data imageprovider.ImageData) error {
	ip.ImageHandler.RemoveImage(imageKey(data))
	return nil
}
