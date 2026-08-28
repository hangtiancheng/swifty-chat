// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package mcp

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// Determines how MCP tools enter the context. Three paths, decided once at
// session start after connecting to MCP:
//
//	eager    — total MCP schema size is under 10% of context; load everything
//	           into tools[] with no deferral. The small context savings are not
//	           worth any additional risk.
//	native   — official Anthropic endpoint. Tools stay in tools[] with
//	           defer_loading but the server hides them from the model;
//	           ToolSearch returns tool_reference so the server expands the schema.
//	Endpoints other than native do not support either mechanism and must
//	simulate it: MCP tools never enter tools[] and calls go through McpCall.
//
// Why three paths: tools are rendered after system and before messages; any
// array change invalidates the entire trailing conversation history cache.
// Measured with 20k-token history, appending one tool to tools drops the hit
// rate from 99.4% to 9.5% — effectively recomputing the entire history.
const (
	// DefaultEagerThresholdPercent: below this fraction of the context window, skip deferral
	DefaultEagerThresholdPercent = 10

	// CharsPerToken is the estimation ratio used when real token counts are
	// unavailable. MCP schemas are JSON with high symbol density, so they
	// have fewer characters per token than natural language.
	CharsPerToken = 2.5

	// NativeToolSearchBeta is the beta header for the official endpoint;
	// both defer_loading and tool_reference require it.
	NativeToolSearchBeta = "advanced-tool-use-2025-11-20"

	envLoadingOverride = "SWIFTX_MCP_LOADING"
)

var officialHosts = map[string]bool{"api.anthropic.com": true}

// IsOfficialAnthropicEndpoint reports whether the endpoint is official.
// An empty baseURL means the SDK default address, which is official.
func IsOfficialAnthropicEndpoint(baseURL string) bool {
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return officialHosts[strings.ToLower(u.Hostname())]
}

// EstimateSchemaTokens estimates token count from character count.
func EstimateSchemaTokens(schemaChars int) int {
	return int(float64(schemaChars) / CharsPerToken)
}

// DecideMode determines the loading mode.
func DecideMode(baseURL string, contextWindow, mcpSchemaChars, thresholdPercent int) tools.McpLoadingMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envLoadingOverride))) {
	case "eager":
		return tools.McpLoadingEager
	case "native":
		return tools.McpLoadingNative
	case "dispatch":
		return tools.McpLoadingDispatch
	}

	// No MCP tools; any path behaves the same, eager is simplest
	if mcpSchemaChars <= 0 {
		return tools.McpLoadingEager
	}

	budget := float64(contextWindow) * float64(thresholdPercent) / 100
	if float64(EstimateSchemaTokens(mcpSchemaChars)) < budget {
		return tools.McpLoadingEager
	}
	if IsOfficialAnthropicEndpoint(baseURL) {
		return tools.McpLoadingNative
	}
	return tools.McpLoadingDispatch
}

// MeasureSchemaChars counts the serialized character length of MCP tool
// schemas, used for threshold comparison.
func MeasureSchemaChars(registry *tools.Registry) int {
	total := 0
	for _, t := range registry.ListTools() {
		if !strings.HasPrefix(t.Name(), tools.MCPToolPrefix) {
			continue
		}
		if b, err := json.Marshal(t.Schema()); err == nil {
			total += len(b)
		} else {
			total += len(t.Name()) + len(t.Description())
		}
	}
	return total
}

// ApplyMode applies the decision to the registry.
//
// Under eager, the defer flag on MCP tools is cleared so they appear in
// tools[]; the other two paths keep them deferred. McpCall is not registered
// here — it must already be in tools[] before the MCP connection is
// established; adding it after connection would be a mid-session tools array
// change, breaking the cache just the same.
func ApplyMode(registry *tools.Registry, mode tools.McpLoadingMode) {
	registry.McpLoadingMode = mode
	eager := mode == tools.McpLoadingEager
	for _, t := range registry.ListTools() {
		if mt, ok := t.(tools.MCPTool); ok {
			mt.SetDeferLoading(!eager)
		}
	}

	// Search and dispatch exposure is decided by mode. Under eager all tools
	// are in tools[]; there is nothing to search and no dispatch entry point
	// needed. These two flags are computed once here and fixed for the entire
	// session, causing no mid-session tools[] churn.
	registry.ExposeToolSearch = !eager
	registry.ExposeMcpCall = mode == tools.McpLoadingDispatch
}

// DecideAndApply is the single entry point called after connecting to MCP.
func DecideAndApply(registry *tools.Registry, baseURL string, contextWindow int) tools.McpLoadingMode {
	mode := DecideMode(baseURL, contextWindow, MeasureSchemaChars(registry), DefaultEagerThresholdPercent)
	ApplyMode(registry, mode)
	return mode
}
