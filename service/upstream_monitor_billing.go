package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func monitorBoundModelName(binding MonitorBinding, channel model.Channel, localModel string) string {
	if binding.UpstreamModel != "" {
		return binding.UpstreamModel
	}
	if channel.ModelMapping != nil {
		var mappings map[string]string
		if common.UnmarshalJsonStr(*channel.ModelMapping, &mappings) == nil && mappings[localModel] != "" {
			return mappings[localModel]
		}
	}
	return localModel
}
