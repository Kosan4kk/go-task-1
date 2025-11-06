package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type PodSpec struct {
	OS         string      `yaml:"os"`
	Containers []Container `yaml:"containers"`
}

type Container struct {
	Name      string    `yaml:"name"`
	Image     string    `yaml:"image"`
	Ports     []Port    `yaml:"ports"`
	Resources Resources `yaml:"resources"`
}

type Port struct {
	ContainerPort int    `yaml:"containerPort"`
	Protocol      string `yaml:"protocol"`
}

type Resources struct {
	Limits   ResourceList `yaml:"limits"`
	Requests ResourceList `yaml:"requests"`
}

type ResourceList struct {
	CPU    interface{} `yaml:"cpu"`
	Memory string      `yaml:"memory"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Pod struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       PodSpec  `yaml:"spec"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <yaml-file>\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	filename := os.Args[1]
	if err := validateYAML(filename); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func validateYAML(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("%s: cannot read file: %v", filename, err)
	}

	// First pass: parse with detailed node information to get line numbers
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return fmt.Errorf("%s: cannot unmarshal YAML: %v", filename, err)
	}

	// Collect all validation errors
	var errors []string

	// Find and validate spec.os
	if osNode := findNode(&node, "spec", "os"); osNode != nil {
		if osNode.Value != "linux" && osNode.Value != "windows" {
			errors = append(errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
		}
	}

	// Find and validate containers
	containersNode := findNode(&node, "spec", "containers")
	if containersNode != nil && containersNode.Kind == yaml.SequenceNode {
		for i := 0; i < len(containersNode.Content); i++ {
			containerNode := containersNode.Content[i]
			validateContainer(containerNode, filename, &errors)
		}
	}

	// Second pass: validate basic structure with typed struct
	var pod Pod
	if err := yaml.Unmarshal(content, &pod); err != nil {
		return fmt.Errorf("%s: cannot unmarshal YAML: %v", filename, err)
	}

	// Basic validations
	if pod.APIVersion != "v1" {
		return fmt.Errorf("apiVersion is required and must be 'v1'")
	}

	if pod.Kind != "Pod" {
		return fmt.Errorf("kind is required and must be 'Pod'")
	}

	if pod.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if len(pod.Spec.Containers) == 0 {
		return fmt.Errorf("spec.containers is required and must contain at least one container")
	}

	// Return all collected errors if any
	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}

	return nil
}

func validateContainer(containerNode *yaml.Node, filename string, errors *[]string) {
	// Validate container name
	nameNode := findNodeInMapping(containerNode, "name")
	if nameNode != nil {
		if nameNode.Value == "" {
			*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
		}
	}

	// Validate container ports
	portsNode := findNodeInMapping(containerNode, "ports")
	if portsNode != nil && portsNode.Kind == yaml.SequenceNode {
		for i := 0; i < len(portsNode.Content); i++ {
			portNode := portsNode.Content[i]
			validateContainerPort(portNode, filename, errors)
		}
	}

	// Validate CPU resources
	validateContainerCPU(containerNode, filename, errors)
}

func validateContainerPort(portNode *yaml.Node, filename string, errors *[]string) {
	containerPortNode := findNodeInMapping(portNode, "containerPort")
	if containerPortNode != nil && containerPortNode.Kind == yaml.ScalarNode {
		port, err := strconv.Atoi(containerPortNode.Value)
		if err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort must be integer", filename, containerPortNode.Line))
		} else if port < 1 || port > 65535 {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, containerPortNode.Line))
		}
	}
}

func validateContainerCPU(containerNode *yaml.Node, filename string, errors *[]string) {
	resourcesNode := findNodeInMapping(containerNode, "resources")
	if resourcesNode == nil {
		return
	}

	limitsNode := findNodeInMapping(resourcesNode, "limits")
	if limitsNode != nil {
		cpuNode := findNodeInMapping(limitsNode, "cpu")
		if cpuNode != nil && cpuNode.Kind == yaml.ScalarNode {
			if _, err := strconv.Atoi(cpuNode.Value); err != nil {
				*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
			}
		}
	}

	requestsNode := findNodeInMapping(resourcesNode, "requests")
	if requestsNode != nil {
		cpuNode := findNodeInMapping(requestsNode, "cpu")
		if cpuNode != nil && cpuNode.Kind == yaml.ScalarNode {
			if _, err := strconv.Atoi(cpuNode.Value); err != nil {
				*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
			}
		}
	}
}

func findNode(root *yaml.Node, path ...string) *yaml.Node {
	if root == nil || len(root.Content) == 0 {
		return nil
	}

	current := root.Content[0] // Document node
	for _, segment := range path {
		found := false
		if current.Kind == yaml.MappingNode {
			for i := 0; i < len(current.Content); i += 2 {
				key := current.Content[i]
				value := current.Content[i+1]
				if key.Value == segment {
					current = value
					found = true
					break
				}
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

func findNodeInMapping(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valueNode := mapping.Content[i+1]
		if keyNode.Value == key {
			return valueNode
		}
	}
	return nil
}
