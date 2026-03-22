package db

import (
	"testing"
)

func TestComputeBootConfigHash(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		wantEmpty  bool
	}{
		{
			name:       "empty string returns empty",
			configJSON: "",
			wantEmpty:  true,
		},
		{
			name:       "empty object returns empty",
			configJSON: "{}",
			wantEmpty:  true,
		},
		{
			name:       "config with no startup_script returns empty",
			configJSON: `{"libvirt":{"uri":"qemu:///system","network":"default"}}`,
			wantEmpty:  true,
		},
		{
			name:       "config with libvirt startupScript returns non-empty",
			configJSON: `{"libvirt":{"uri":"qemu:///system","startupScript":"#!/bin/bash\necho hello"}}`,
			wantEmpty:  false,
		},
		{
			name:       "config with gce startup_script snake_case returns non-empty",
			configJSON: `{"gce":{"project":"my-project","startup_script":"#!/bin/bash\necho hello"}}`,
			wantEmpty:  false,
		},
		{
			name:       "config with lxd startupScript returns non-empty",
			configJSON: `{"lxd":{"endpoint":"https://localhost:8443","startupScript":"#!/bin/bash\necho hello"}}`,
			wantEmpty:  false,
		},
		{
			name:       "invalid JSON returns empty",
			configJSON: `not json`,
			wantEmpty:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeBootConfigHash(tc.configJSON)
			if tc.wantEmpty && got != "" {
				t.Errorf("expected empty hash, got %q", got)
			}
			if !tc.wantEmpty && got == "" {
				t.Errorf("expected non-empty hash, got empty")
			}
		})
	}
}

func TestComputeBootConfigHash_Deterministic(t *testing.T) {
	config := `{"libvirt":{"startupScript":"#!/bin/bash\necho hello"}}`
	hash1 := computeBootConfigHash(config)
	hash2 := computeBootConfigHash(config)
	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %q and %q", hash1, hash2)
	}
}

func TestComputeBootConfigHash_DifferentScripts(t *testing.T) {
	config1 := `{"libvirt":{"startupScript":"#!/bin/bash\necho hello"}}`
	config2 := `{"libvirt":{"startupScript":"#!/bin/bash\necho world"}}`
	hash1 := computeBootConfigHash(config1)
	hash2 := computeBootConfigHash(config2)
	if hash1 == hash2 {
		t.Errorf("expected different hashes for different scripts, got same: %q", hash1)
	}
}

func TestComputeBootConfigHash_WithAndWithoutScript(t *testing.T) {
	withScript := `{"libvirt":{"uri":"qemu:///system","startupScript":"#!/bin/bash\necho hello"}}`
	withoutScript := `{"libvirt":{"uri":"qemu:///system"}}`
	hashWith := computeBootConfigHash(withScript)
	hashWithout := computeBootConfigHash(withoutScript)
	if hashWith == "" {
		t.Fatal("expected non-empty hash for config with script")
	}
	if hashWithout != "" {
		t.Fatal("expected empty hash for config without script")
	}
}
