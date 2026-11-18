package setting

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var Chats = []map[string]any{
	//{
	//	"ChatGPT Next Web 官方示例": "https://app.nextchat.dev/#/?settings={\"key\":\"{key}\",\"url\":\"{address}\"}",
	//},
	{
		"Cherry Studio": "cherrystudio://providers/api-keys?v=1&data={cherryConfig}",
	},
	{
		"AionUI": "aionui://provider/add?v=1&data={aionuiConfig}",
	},
	{
		"流畅阅读": "fluentread",
	},
	{
		"CC Switch": "ccswitch",
	},
	{
		"DeepChat": "deepchat://provider/install?v=1&data={deepchatConfig}",
	},
	{
		"AQBot": "aqbot://providers?{aqbotConfig}",
	},
	{
		"Lobe Chat 官方示例": "https://chat-preview.lobehub.com/?settings={\"keyVaults\":{\"openai\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\"}}}",
	},
	{
		"AI as Workspace": "https://aiaw.app/set-provider?provider={\"type\":\"openai\",\"settings\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\",\"compatibility\":\"strict\"}}",
	},
	{
		"AMA 问天": "ama://set-api-key?server={address}&key={key}",
	},
	{
		"OpenCat": "opencat://team/join?domain={address}&token={key}",
	},
}

// ParseChats also accepts the original single name-to-URL mapping.
func ParseChats(value string) ([]map[string]any, error) {
	var chats []map[string]any
	if err := common.UnmarshalJsonStr(value, &chats); err != nil {
		return nil, err
	}
	if chats == nil {
		return nil, errors.New("Expected a JSON array.")
	}
	invalid := errors.New("Invalid chat preset.")
	shortcuts := 0
	for _, entry := range chats {
		if len(entry) == 1 {
			for name, raw := range entry {
				link, ok := raw.(string)
				if strings.TrimSpace(name) == "" || !ok || strings.TrimSpace(link) == "" {
					return nil, invalid
				}
			}
			continue
		}
		name, _ := entry["name"].(string)
		link, _ := entry["url"].(string)
		if strings.TrimSpace(name) == "" || strings.TrimSpace(link) == "" {
			return nil, invalid
		}
		for key, raw := range entry {
			switch key {
			case "name", "url":
			case "icon":
				if _, ok := raw.(string); !ok {
					return nil, invalid
				}
			case "shortcut":
				enabled, ok := raw.(bool)
				if !ok {
					return nil, invalid
				}
				if enabled {
					shortcuts++
				}
			default:
				return nil, invalid
			}
		}
	}
	if shortcuts > 3 {
		return nil, errors.New("Select up to 3 shortcuts.")
	}
	return chats, nil
}

func UpdateChatsByJsonString(value string) error {
	chats, err := ParseChats(value)
	if err == nil {
		Chats = chats
	}
	return err
}

func Chats2JsonString() string {
	jsonBytes, err := common.Marshal(Chats)
	if err != nil {
		common.SysLog(common.LogText("error marshalling chats: %s", err.Error()))
		return "[]"
	}
	return string(jsonBytes)
}
