package ignition

import (
	ignition_config_types_32 "github.com/coreos/ignition/v2/config/v3_2/types"
	"net/url"
	"sigs.k8s.io/yaml"
)

type nmstateOutput struct {
	NetworkManager [][]string `yaml:"NetworkManager"`
}

func nmstateOutputToFiles(generatedConfig []byte) ([]ignition_config_types_32.File, error) {
	files := []ignition_config_types_32.File{}

	networkManagerConfig := &nmstateOutput{}
	err := yaml.Unmarshal(generatedConfig, networkManagerConfig)
	if err != nil {
		return nil, err
	}
	if networkManagerConfig.NetworkManager == nil {
		return files, nil
	}
	for _, v := range networkManagerConfig.NetworkManager {
		name := v[0]
		source := "data:," + url.QueryEscape(v[1])
		files = append(files, ignition_config_types_32.File{
			Node:          ignition_config_types_32.Node{Path: "/etc/NetworkManager/system-connections/" + name},
			FileEmbedded1: ignition_config_types_32.FileEmbedded1{Contents: ignition_config_types_32.Resource{Source: &source}},
		})
	}
	return files, nil
}
