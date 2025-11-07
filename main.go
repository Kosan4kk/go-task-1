package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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

	// Получаем корневую ноду
	if len(node.Content) == 0 {
		fmt.Fprintf(os.Stderr, "%s:1 empty document\n", filename)
		os.Exit(1)
	}
	doc := node.Content[0]
	if doc.Kind != yaml.MappingNode {
		fmt.Fprintf(os.Stderr, "%s:1 root must be mapping\n", filename)
		os.Exit(1)
	}

	// Проверяем обязательные поля верхнего уровня
	checkRequiredField(doc, "apiVersion", filename, &errors)
	checkRequiredField(doc, "kind", filename, &errors)
	checkRequiredField(doc, "metadata", filename, &errors)
	checkRequiredField(doc, "spec", filename, &errors)

	// Проверяем значения полей
	checkApiVersion(doc, filename, &errors)
	checkKind(doc, filename, &errors)
	checkMetadata(doc, filename, &errors)
	checkSpec(doc, filename, &errors)

	if len(errors) > 0 {
		// ВЫВОДИМ ВСЕ ОШИБКИ В STDERR
		for _, err := range errors {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func checkRequiredField(doc *yaml.Node, field string, filename string, errors *[]string) {
	if getField(doc, field) == nil {
		*errors = append(*errors, fmt.Sprintf("%s:1 %s is required", filename, field))
	}
}

func checkApiVersion(doc *yaml.Node, filename string, errors *[]string) {
	apiNode := getField(doc, "apiVersion")
	if apiNode == nil {
		return
	}
	if apiNode.Value != "v1" {
		*errors = append(*errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, apiNode.Line))
	}
}

func checkKind(doc *yaml.Node, filename string, errors *[]string) {
	kindNode := getField(doc, "kind")
	if kindNode == nil {
		return
	}
	if kindNode.Value != "Pod" {
		*errors = append(*errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, kindNode.Line))
	}
}

func checkMetadata(doc *yaml.Node, filename string, errors *[]string) {
	metaNode := getField(doc, "metadata")
	if metaNode == nil {
		return
	}
	if metaNode.Kind != yaml.MappingNode {
		*errors = append(*errors, fmt.Sprintf("%s:%d metadata must be mapping", filename, metaNode.Line))
		return
	}

	nameNode := getField(metaNode, "name")
	if nameNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:%d metadata.name is required", filename, metaNode.Line))
	} else if nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}
}

func checkSpec(doc *yaml.Node, filename string, errors *[]string) {
	specNode := getField(doc, "spec")
	if specNode == nil {
		return
	}
	if specNode.Kind != yaml.MappingNode {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec must be mapping", filename, specNode.Line))
		return
	}

	// Проверяем os
	osNode := getField(specNode, "os")
	if osNode != nil {
		if osNode.Value != "linux" && osNode.Value != "windows" {
			*errors = append(*errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
		}
	}

	// Проверяем containers
	containersNode := getField(specNode, "containers")
	if containersNode == nil {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers is required", filename, specNode.Line))
		return
	}

	if containersNode.Kind != yaml.SequenceNode {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers must be sequence", filename, containersNode.Line))
		return
	}

	if len(containersNode.Content) == 0 {
		*errors = append(*errors, fmt.Sprintf("%s:%d spec.containers must contain at least one container", filename, containersNode.Line))
		return
	}

	// Проверяем каждый контейнер
	for i, containerNode := range containersNode.Content {
		if containerNode.Kind != yaml.MappingNode {
			*errors = append(*errors, fmt.Sprintf("%s:%d container must be mapping", filename, containerNode.Line))
			continue
		}

		checkContainer(containerNode, filename, i, errors)
	}
}

func checkContainer(container *yaml.Node, filename string, index int, errors *[]string) {
	// Проверяем имя
	nameNode := getField(container, "name")
	if nameNode != nil && nameNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	}

	// Проверяем image
	imageNode := getField(container, "image")
	if imageNode != nil && imageNode.Value == "" {
		*errors = append(*errors, fmt.Sprintf("%s:%d image is required", filename, imageNode.Line))
	}

	// Проверяем ports
	portsNode := getField(container, "ports")
	if portsNode != nil && portsNode.Kind == yaml.SequenceNode {
		for _, portNode := range portsNode.Content {
			if portNode.Kind == yaml.MappingNode {
				checkContainerPort(portNode, filename, errors)
			}
		}
	}

	// Проверяем resources
	resourcesNode := getField(container, "resources")
	if resourcesNode != nil && resourcesNode.Kind == yaml.MappingNode {
		checkResources(resourcesNode, filename, errors)
	}
}

func checkContainerPort(port *yaml.Node, filename string, errors *[]string) {
	portNode := getField(port, "containerPort")
	if portNode != nil {
		portVal, err := strconv.Atoi(portNode.Value)
		if err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort must be integer", filename, portNode.Line))
		} else if portVal < 1 || portVal > 65535 {
			*errors = append(*errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, portNode.Line))
		}
	}
}

func checkResources(resources *yaml.Node, filename string, errors *[]string) {
	limitsNode := getField(resources, "limits")
	if limitsNode != nil && limitsNode.Kind == yaml.MappingNode {
		checkCPU(limitsNode, filename, errors)
	}

	requestsNode := getField(resources, "requests")
	if requestsNode != nil && requestsNode.Kind == yaml.MappingNode {
		checkCPU(requestsNode, filename, errors)
	}
}

func checkCPU(resource *yaml.Node, filename string, errors *[]string) {
	cpuNode := getField(resource, "cpu")
	if cpuNode != nil {
		if _, err := strconv.Atoi(cpuNode.Value); err != nil {
			*errors = append(*errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
		}
	}
}

func getField(node *yaml.Node, field string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Value == field {
			return value
		}
	}
	return nil
}
