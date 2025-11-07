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

	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return fmt.Errorf("%s: cannot unmarshal YAML: %v", filename, err)
	}

	var errors []string

	validateBasicStructure(&node, filename, &errors)

	containersNode := findNode(&node, "spec", "containers")
	if containersNode != nil && containersNode.Kind == yaml.SequenceNode {
		for i := 0; i < len(containersNode.Content); i++ {
			containerNode := containersNode.Content[i]
			validateContainer(containerNode, filename, &errors)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}

	return nil
}

func validateBasicStructure(root *yaml.Node, filename string, errors *[]string) {
	apiVersionNode := findNode(root, "apiVersion")
	if apiVersionNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 apiVersion is required", filename))
	} else if apiVersionNode.Value != "v1" {
		*errors = append(*errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, apiVersionNode.Line))
	}

	kindNode := findNode(root, "kind")
	if kindNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 kind is required", filename))
	} else if kindNode.Value != "Pod" {
		*errors = append(*errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, kindNode.Line))
	}

	nameNode := findNode(root, "metadata", "name")
	if nameNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 metadata.name is required", filename))
	} else if nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	osNode := findNode(root, "spec", "os")
	if osNode != nil {
		if osNode.Value != "linux" && osNode.Value != "windows" {
			*errors = append(*errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
		}
	}

	containersNode := findNode(root, "spec", "containers")
	if containersNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 spec.containers is required", filename))
	} else if containersNode.Kind == yaml.SequenceNode && len(containersNode.Content) == 0 {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers must contain at least one container", filename, containersNode.Line))
	}
}

func validateContainer(containerNode *yaml.Node, filename string, errors *[]string) {
	nameNode := findNodeInMapping(containerNode, "name")
	if nameNode != nil && nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	imageNode := findNodeInMapping(containerNode, "image")
	if imageNode != nil && imageNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d image is required", filename, imageNode.Line))
	}

	portsNode := findNodeInMapping(containerNode, "ports")
	if portsNode != nil && portsNode.Kind == yaml.SequenceNode {
		for i := 0; i < len(portsNode.Content); i++ {
			portNode := portsNode.Content[i]
			validateContainerPort(portNode, filename, errors)
		}
	}

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

	validateCPUInResources(resourcesNode, "limits", filename, errors)
	validateCPUInResources(resourcesNode, "requests", filename, errors)
}

func validateCPUInResources(resourcesNode *yaml.Node, resourceType string, filename string, errors *[]string) {
	resourceNode := findNodeInMapping(resourcesNode, resourceType)
	if resourceNode == nil {
		return
	}

	cpuNode := findNodeInMapping(resourceNode, "cpu")
	if cpuNode != nil && cpuNode.Kind == yaml.ScalarNode {
		if _, err := strconv.Atoi(cpuNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
		}
	}
}

func findNode(root *yaml.Node, path ...string) *yaml.Node {
	if root == nil || len(root.Content) == 0 {
		return nil
	}

	current := root.Content[0]
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
