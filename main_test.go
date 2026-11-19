package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// TestPluginDescribe verifies the actual entry point registers a default
// provisioner and advertises the SDK protocol and stable version.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestPluginDescribe(t *testing.T) {
	output, err := exec.Command("go", "run", ".", "describe").Output()
	if err != nil {
		t.Fatal(err)
	}
	var description struct {
		Version      string   `json:"version"`
		APIVersion   string   `json:"api_version"`
		Provisioners []string `json:"provisioners"`
	}
	if err := json.Unmarshal(output, &description); err != nil {
		t.Fatal(err)
	}
	if description.Version != "1.0.0" || description.APIVersion == "" || len(description.Provisioners) != 1 || description.Provisioners[0] != "-packer-default-plugin-name-" {
		t.Fatalf("unexpected plugin description: %s", output)
	}
}
