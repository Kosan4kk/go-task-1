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

	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot unmarshal YAML\n", filename)
		os.Exit(1)
	}

	var errors []string

	// Проверка обязательных полей
	if data["apiVersion"] != "v1" {
		errors = append(errors, "apiVersion must be 'v1'")
	}
	if data["kind"] != "Pod" {
		errors = append(errors, "kind must be 'Pod'")
	}

	// Проверка metadata.name
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); !ok || name == "" {
			errors = append(errors, "metadata.name is required")
		}
	} else {
		errors = append(errors, "metadata.name is required")
	}

	// Проверка spec
	if spec, ok := data["spec"].(map[string]interface{}); ok {
		// Проверка os
		if osVal, ok := spec["os"].(string); ok {
			if osVal != "linux" && osVal != "windows" {
				errors = append(errors, fmt.Sprintf("os has unsupported value '%s'", osVal))
			}
		}

		// Проверка containers
		if containers, ok := spec["containers"].([]interface{}); ok {
			if len(containers) == 0 {
				errors = append(errors, "spec.containers must contain at least one container")
			}

			for _, container := range containers {
				if cont, ok := container.(map[string]interface{}); ok {
					// Проверка имени контейнера
					if name, ok := cont["name"].(string); !ok || name == "" {
						errors = append(errors, "name is required")
					}

					// Проверка image
					if image, ok := cont["image"].(string); !ok || image == "" {
						errors = append(errors, "image is required")
					}

					// Проверка resources
					if resources, ok := cont["resources"].(map[string]interface{}); ok {
						// Проверка limits
						if limits, ok := resources["limits"].(map[string]interface{}); ok {
							if cpu, exists := limits["cpu"]; exists {
								if _, err := strconv.Atoi(fmt.Sprintf("%v", cpu)); err != nil {
									errors = append(errors, "cpu must be int")
								}
							}
						}

						// Проверка requests
						if requests, ok := resources["requests"].(map[string]interface{}); ok {
							if cpu, exists := requests["cpu"]; exists {
								if _, err := strconv.Atoi(fmt.Sprintf("%v", cpu)); err != nil {
									errors = append(errors, "cpu must be int")
								}
							}
						}
					}

					// Проверка ports
					if ports, ok := cont["ports"].([]interface{}); ok {
						for _, port := range ports {
							if portMap, ok := port.(map[string]interface{}); ok {
								if containerPort, exists := portMap["containerPort"]; exists {
									portVal, err := strconv.Atoi(fmt.Sprintf("%v", containerPort))
									if err != nil {
										errors = append(errors, "containerPort must be integer")
									} else if portVal < 1 || portVal > 65535 {
										errors = append(errors, "containerPort value out of range")
									}
								}
							}
						}
					}
				}
			}
		} else {
			errors = append(errors, "spec.containers is required")
		}
	} else {
		errors = append(errors, "spec.containers is required")
	}

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Join(errors, "\n"))
		os.Exit(1)
	}
}
