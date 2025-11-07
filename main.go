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

	// Проходим по всем нодам и собираем ошибки
	var errors []string
	collectErrors(&node, filename, &errors)

	// Выводим все ошибки
	for _, err := range errors {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(errors) > 0 {
		os.Exit(1)
	}
}

func collectErrors(node *yaml.Node, filename string, errors *[]string) {
	if node.Kind != yaml.DocumentNode {
		return
	}

	doc := node.Content[0]
	if doc.Kind != yaml.MappingNode {
		return
	}

	// Проверяем apiVersion
	apiNode := findNode(doc, "apiVersion")
	if apiNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 apiVersion is required", filename))
	} else if apiNode.Value != "v1" {
		*errors = append(*errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, apiNode.Line))
	}

	// Проверяем kind
	kindNode := findNode(doc, "kind")
	if kindNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 kind is required", filename))
	} else if kindNode.Value != "Pod" {
		*errors = append(*errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, kindNode.Line))
	}

	// Проверяем metadata.name
	metaNode := findNode(doc, "metadata")
	if metaNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 metadata is required", filename))
	} else {
		nameNode := findNode(metaNode, "name")
		if nameNode == nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d metadata.name is required", filename, metaNode.Line))
		} else if nameNode.Value == "" {
			*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
		}
	}

	// Проверяем spec
	specNode := findNode(doc, "spec")
	if specNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 spec is required", filename))
	} else {
		// Проверяем os
		osNode := findNode(specNode, "os")
		if osNode != nil {
			if osNode.Value != "linux" && osNode.Value != "windows" {
				*errors = append(*errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
			}
		}

		// Проверяем containers
		containersNode := findNode(specNode, "containers")
		if containersNode == nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers is required", filename, specNode.Line))
		} else if len(containersNode.Content) == 0 {
			*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers must contain at least one container", filename, containersNode.Line))
		} else {
			for _, container := range containersNode.Content {
				checkContainer(container, filename, errors)
			}
		}
	}
}

func checkContainer(container *yaml.Node, filename string, errors *[]string) {
	// Проверяем имя контейнера
	nameNode := findNode(container, "name")
	if nameNode != nil && nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	// Проверяем image
	imageNode := findNode(container, "image")
	if imageNode != nil && imageNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d image is required", filename, imageNode.Line))
	}

	// Проверяем ports
	portsNode := findNode(container, "ports")
	if portsNode != nil {
		for _, port := range portsNode.Content {
			checkPort(port, filename, errors)
		}
	}

	// Проверяем readinessProbe
	readinessNode := findNode(container, "readinessProbe")
	if readinessNode != nil {
		checkProbe(readinessNode, filename, errors)
	}

	// Проверяем livenessProbe
	livenessNode := findNode(container, "livenessProbe")
	if livenessNode != nil {
		checkProbe(livenessNode, filename, errors)
	}

	// Проверяем resources
	resourcesNode := findNode(container, "resources")
	if resourcesNode != nil {
		checkResources(resourcesNode, filename, errors)
	}
}

func checkPort(port *yaml.Node, filename string, errors *[]string) {
	portNode := findNode(port, "containerPort")
	if portNode != nil {
		if val, err := strconv.Atoi(portNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort must be integer", filename, portNode.Line))
		} else if val < 1 || val > 65535 {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, portNode.Line))
		}
	}
}

func checkProbe(probe *yaml.Node, filename string, errors *[]string) {
	httpGetNode := findNode(probe, "httpGet")
	if httpGetNode != nil {
		portNode := findNode(httpGetNode, "port")
		if portNode != nil {
			if val, err := strconv.Atoi(portNode.Value); err != nil {
				*errors = append(*errors, fmt.Sprintf("%s:%d port must be integer", filename, portNode.Line))
			} else if val < 1 || val > 65535 {
				*errors = append(*errors, fmt.Sprintf("%s:%d port value out of range", filename, portNode.Line))
			}
		}
	}
}

func checkResources(resources *yaml.Node, filename string, errors *[]string) {
	limitsNode := findNode(resources, "limits")
	if limitsNode != nil {
		checkCPU(limitsNode, filename, errors)
	}

	requestsNode := findNode(resources, "requests")
	if requestsNode != nil {
		checkCPU(requestsNode, filename, errors)
	}
}

func checkCPU(resource *yaml.Node, filename string, errors *[]string) {
	cpuNode := findNode(resource, "cpu")
	if cpuNode != nil {
		if _, err := strconv.Atoi(cpuNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
		}
	}
}

func findNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Value == key {
			return v
		}
	}
	return nil
}
