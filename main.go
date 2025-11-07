package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <yaml-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := os.Args[1]
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s:1 cannot read file\n", filename)
		os.Exit(1)
	}

	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		fmt.Fprintf(os.Stderr, "%s:1 cannot unmarshal YAML\n", filename)
		os.Exit(1)
	}

	var errors []string
	doc := node.Content[0]

	// Проверка обязательных полей
	checkRequiredFields(doc, filename, &errors)
	
	// Проверка spec и containers
	if spec := findNode(doc, "spec"); spec != nil {
		checkSpec(spec, filename, &errors)
	}

	// Выводим все ошибки
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func checkRequiredFields(doc *yaml.Node, filename string, errors *[]string) {
	// apiVersion
	if apiNode := findNode(doc, "apiVersion"); apiNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 apiVersion is required", filename))
	} else if apiNode.Value != "v1" {
		*errors = append(*errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, apiNode.Line))
	}

	// kind
	if kindNode := findNode(doc, "kind"); kindNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 kind is required", filename))
	} else if kindNode.Value != "Pod" {
		*errors = append(*errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, kindNode.Line))
	}

	// metadata.name
	if metaNode := findNode(doc, "metadata"); metaNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 metadata is required", filename))
	} else if nameNode := findNode(metaNode, "name"); nameNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:%d metadata.name is required", filename, metaNode.Line))
	} else if nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	// spec.containers
	if specNode := findNode(doc, "spec"); specNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 spec is required", filename))
	} else if containersNode := findNode(specNode, "containers"); containersNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers is required", filename, specNode.Line))
	} else if len(containersNode.Content) == 0 {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers must contain at least one container", filename, containersNode.Line))
	}
}

func checkSpec(spec *yaml.Node, filename string, errors *[]string) {
	// Проверка os
	if osNode := findNode(spec, "os"); osNode != nil {
		if osNode.Value != "linux" && osNode.Value != "windows" {
			*errors = append(*errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
		}
	}

	// Проверка containers
	if containersNode := findNode(spec, "containers"); containersNode != nil {
		for i, containerNode := range containersNode.Content {
			checkContainer(containerNode, filename, i, errors)
		}
	}
}

func checkContainer(container *yaml.Node, filename string, index int, errors *[]string) {
	// Проверка имени контейнера
	if nameNode := findNode(container, "name"); nameNode != nil && nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	// Проверка image
	if imageNode := findNode(container, "image"); imageNode != nil && imageNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d image is required", filename, imageNode.Line))
	}

	// Проверка ports
	if portsNode := findNode(container, "ports"); portsNode != nil {
		for _, portNode := range portsNode.Content {
			checkContainerPort(portNode, filename, errors)
		}
	}

	// Проверка readinessProbe и livenessProbe
	checkProbe(container, "readinessProbe", filename, errors)
	checkProbe(container, "livenessProbe", filename, errors)

	// Проверка resources
	if resourcesNode := findNode(container, "resources"); resourcesNode != nil {
		checkResources(resourcesNode, filename, errors)
	}
}

func checkContainerPort(port *yaml.Node, filename string, errors *[]string) {
	if portNode := findNode(port, "containerPort"); portNode != nil {
		if portVal, err := strconv.Atoi(portNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort must be integer", filename, portNode.Line))
		} else if portVal < 1 || portVal > 65535 {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, portNode.Line))
		}
	}
}

func checkProbe(container *yaml.Node, probeName string, filename string, errors *[]string) {
	if probeNode := findNode(container, probeName); probeNode != nil {
		if httpGetNode := findNode(probeNode, "httpGet"); httpGetNode != nil {
			if portNode := findNode(httpGetNode, "port"); portNode != nil {
				if portVal, err := strconv.Atoi(portNode.Value); err != nil {
					*errors = append(*errors, fmt.Sprintf("%s:%d port must be integer", filename, portNode.Line))
				} else if portVal < 1 || portVal > 65535 {
					*errors = append(*errors, fmt.Sprintf("%s:%d port value out of range", filename, portNode.Line))
				}
			}
		}
	}
}

func checkResources(resources *yaml.Node, filename string, errors *[]string) {
	if limitsNode := findNode(resources, "limits"); limitsNode != nil {
		checkCPU(limitsNode, filename, errors)
	}
	if requestsNode := findNode(resources, "requests"); requestsNode != nil {
		checkCPU(requestsNode, filename, errors)
	}
}

func checkCPU(resource *yaml.Node, filename string, errors *[]string) {
	if cpuNode := findNode(resource, "cpu"); cpuNode != nil {
		if _, err := strconv.Atoi(cpuNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
		}
	}
}

func findNode(node *yaml.Node, path ...string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	current := node
	for _, segment := range path {
		found := false
		for i := 0; i < len(current.Content); i += 2 {
			key := current.Content[i]
			value := current.Content[i+1]
			if key.Value == segment {
				current = value
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}
