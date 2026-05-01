package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type ClaudeMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

func loadClaudeMCPServers(cwd string) map[string]json.RawMessage {
	merged := make(map[string]json.RawMessage)

	home, err := os.UserHomeDir()
	if err != nil {
		return merged
	}

	globalPath := filepath.Join(home, ".claude", ".mcp.json")
	loadMCPFile(globalPath, merged)

	projectPath := filepath.Join(home, ".claude", "projects", "-"+strings.ReplaceAll(cwd, "/", "-"), ".mcp.json")
	loadMCPFile(projectPath, merged)

	return merged
}

func loadMCPFile(path string, dest map[string]json.RawMessage) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg ClaudeMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		debugf("debug: failed to parse claude mcp config %s: %v", path, err)
		return
	}
	for name, server := range cfg.MCPServers {
		dest[name] = server
		debugf("debug: loaded claude mcp server: %s from %s", name, path)
	}
}

func mergeClaudeMCPIntoAgent(agent string, claudeMCP map[string]json.RawMessage) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	agentPath := filepath.Join(home, ".kiro", "agents", agent+".json")

	data, err := os.ReadFile(agentPath)
	if err != nil {
		debugf("debug: cannot read agent config for MCP merge: %v", err)
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		debugf("debug: cannot parse agent config for MCP merge: %v", err)
		return
	}

	existing := make(map[string]json.RawMessage)
	if mcpRaw, ok := raw["mcpServers"]; ok {
		json.Unmarshal(mcpRaw, &existing)
	}

	for name, cfg := range claudeMCP {
		existing[name] = cfg
		debugf("debug: synced claude mcp server: %s", name)
	}

	mcpData, _ := json.Marshal(existing)
	raw["mcpServers"] = mcpData

	var tools []string
	if toolsRaw, ok := raw["tools"]; ok {
		json.Unmarshal(toolsRaw, &tools)
	}
	var allowedTools []string
	if atRaw, ok := raw["allowedTools"]; ok {
		json.Unmarshal(atRaw, &allowedTools)
	}

	toolSet := make(map[string]bool)
	for _, t := range tools {
		toolSet[t] = true
	}
	atSet := make(map[string]bool)
	for _, t := range allowedTools {
		atSet[t] = true
	}

	for name := range claudeMCP {
		ref := "@" + name
		if !toolSet[ref] {
			tools = append(tools, ref)
		}
		if !atSet[ref] {
			allowedTools = append(allowedTools, ref)
		}
	}

	toolsData, _ := json.Marshal(tools)
	raw["tools"] = toolsData
	atData, _ := json.Marshal(allowedTools)
	raw["allowedTools"] = atData

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		debugf("debug: cannot marshal merged agent config: %v", err)
		return
	}
	if err := os.WriteFile(agentPath, out, 0644); err != nil {
		debugf("debug: cannot write merged agent config: %v", err)
		return
	}
	log.Printf("synced %d claude MCP servers into %s", len(claudeMCP), agentPath)
}
