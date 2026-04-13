package config

import (
	"os"
	"strings"
)

// PreProcessDashboardConfig controls the request processing pipeline stages.
type PreProcessDashboardConfig struct {
	ForceRawPrompt   *bool `yaml:"force-raw-prompt" json:"force-raw-prompt"`
	EnableJSONParse  *bool `yaml:"enable-json-parse" json:"enable-json-parse"`
	EnableAuxLogic   *bool `yaml:"enable-aux-logic" json:"enable-aux-logic"`
	EnableTranslator *bool `yaml:"enable-translator" json:"enable-translator"`
}

// LoadEnvOverrides applies any environment variable overrides and establishes defaults.
func (c *PreProcessDashboardConfig) LoadEnvOverrides() {
	if v := os.Getenv("FORCE_RAW_PROMPT"); v != "" {
		val := strings.EqualFold(v, "true") || v == "1"
		c.ForceRawPrompt = &val
	}

	// Default ForceRawPrompt to true if not set
	if c.ForceRawPrompt == nil {
		defaultVal := true
		c.ForceRawPrompt = &defaultVal
	}

	if *c.ForceRawPrompt {
		// When ForceRawPrompt is enabled, all steps must be bypassed.
		f := false
		c.EnableJSONParse = &f
		c.EnableAuxLogic = &f
		c.EnableTranslator = &f
	} else {
		// If ForceRawPrompt is disabled, enable stages by default if not explicitly disabled
		t := true
		if c.EnableJSONParse == nil {
			c.EnableJSONParse = &t
		}
		if c.EnableAuxLogic == nil {
			c.EnableAuxLogic = &t
		}
		if c.EnableTranslator == nil {
			c.EnableTranslator = &t
		}
	}
}

// Ensure ensure pointers are non-nil for safety
func (c *PreProcessDashboardConfig) Ensure() {
	c.LoadEnvOverrides()
}

// Getters 
func (c *PreProcessDashboardConfig) IsForceRawPrompt() bool {
	if c.ForceRawPrompt == nil { return true }
	return *c.ForceRawPrompt
}

func (c *PreProcessDashboardConfig) IsEnableJSONParse() bool {
	if c.EnableJSONParse == nil { return false }
	return *c.EnableJSONParse
}

func (c *PreProcessDashboardConfig) IsEnableAuxLogic() bool {
	if c.EnableAuxLogic == nil { return false }
	return *c.EnableAuxLogic
}

func (c *PreProcessDashboardConfig) IsEnableTranslator() bool {
	if c.EnableTranslator == nil { return false }
	return *c.EnableTranslator
}
