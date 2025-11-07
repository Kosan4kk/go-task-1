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
		fmt.Fprintf(os.Stderr, "%s: cannot read file\n", filename)
		os.Exit(1)
	}

	// Парсим с информацией о нодах для получения номеров строк
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot unmarshal YAML\n", filename)
		os.Exit(1)
	}

	// Парсим как map для простой валидации
	var data map[string]interface{}
	yaml.Unmarshal(content, &data)

	var errors []string

	// Проверка обязательных полей
	if data["apiVersion"] != "v1" {
		errors = append(errors, fmt.Sprintf("%s:1 apiVersion must be 'v1'", filename))
	}
	if data["kind"] != "Pod" {
		errors = append(errors, fmt.Sprintf("%s:1 kind must be 'Pod'", filename))
	}

	// Проверка metadata.name
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); !ok || name == "" {
			// Ищем ноду для получения номера строки
			if nameNode := findNode(&node, "metadata", "name"); nameNode != nil {
				errors = append(errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
			} else {
				errors = append(errors, fmt.Sprintf("%s:1 name is required", filename))
			}
		}
	} else {
		errors = append(errors, fmt.Sprintf("%s:1 metadata.name is required", filename))
	}

	// Проверка spec
	if spec, ok := data["spec"].(map[string]interface{}); ok {
		// Проверка os
		if osVal, ok := spec["os"].(string); ok {
			if osVal != "linux" && osVal != "windows" {
				if osNode := findNode(&node, "spec", "os"); osNode != nil {
					errors = append(errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNode.Line, osVal))
				}
			}
		}

		// Проверка containers
		if containers, ok := spec["containers"].([]interface{}); ok {
			if len(containers) == 0 {
				if containersNode := findNode(&node, "spec", "containers"); containersNode != nil {
					errors = append(errors, fmt.Sprintf("%s:%d spec.containers must contain at least one container", filename, containersNode.Line))
				}
			}

			for i, container := range containers {
				if cont, ok := container.(map[string]interface{}); ok {
					// Проверка имени контейнера
					if name, ok := cont["name"].(string); !ok || name == "" {
						if nameNode := findContainerFieldNode(&node, i, "name"); nameNode != nil {
							errors = append(errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
						}
					}

					// Проверка resources cpu
					if resources, ok := cont["resources"].(map[string]interface{}); ok {
						// Проверка requests cpu
						if requests, ok := resources["requests"].(map[string]interface{}); ok {
							if cpu, exists := requests["cpu"]; exists {
								if _, err := strconv.Atoi(fmt.Sprintf("%v", cpu)); err != nil {
									if cpuNode := findContainerCPUNode(&node, i, "requests", "cpu"); cpuNode != nil {
										errors = append(errors, fmt.Sprintf("%s:%d cpu must be int", filename, cpuNode.Line))
									}
								}
							}
						}
					}
				}
			}
		} else {
			errors = append(errors, fmt.Sprintf("%s:1 spec.containers is required", filename))
		}
	} else {
		errors = append(errors, fmt.Sprintf("%s:1 spec.containers is required", filename))
	}

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Join(errors, "\n"))
		os.Exit(1)
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

func findContainerFieldNode(root *yaml.Node, containerIndex int, field string) *yaml.Node {
	containersNode := findNode(root, "spec", "containers")
	if containersNode == nil || containersNode.Kind != yaml.SequenceNode {
		return nil
	}

	if containerIndex >= len(containersNode.Content) {
		return nil
	}

	containerNode := containersNode.Content[containerIndex]
	if containerNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(containerNode.Content); i += 2 {
		key := containerNode.Content[i]
		value := containerNode.Content[i+1]
		if key.Value == field {
			return value
		}
	}
	return nil
}

func findContainerCPUNode(root *yaml.Node, containerIndex int, resourceType, field string) *yaml.Node {
	resourcesNode := findContainerFieldNode(root, containerIndex, "resources")
	if resourcesNode == nil || resourcesNode.Kind != yaml.MappingNode {
		return nil
	}

	resourceTypeNode := findNodeInMapping(resourcesNode, resourceType)
	if resourceTypeNode == nil || resourceTypeNode.Kind != yaml.MappingNode {
		return nil
	}

	return findNodeInMapping(resourceTypeNode, field)
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
