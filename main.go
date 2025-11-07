package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	snakeCaseRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	memoryRegex    = regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`)
)

type ValidationError struct {
	Line int
	Msg  string
}

func (e ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d %s", os.Args[1], e.Line, e.Msg)
	}
	return fmt.Sprintf("%s %s", os.Args[1], e.Msg)
}

func newValidationError(line int, msg string) error {
	return ValidationError{Line: line, Msg: msg}
}

func reportAllErrors(errors []error) {
	for _, err := range errors {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(errors) > 0 {
		os.Exit(1)
	}
}

func getMapping(node *yaml.Node) map[string]*yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	m := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		m[key.Value] = value
	}
	return m
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: yamlvalid <file.yaml>")
		os.Exit(1)
	}
	filePath := os.Args[1]

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot read file: %v\n", filePath, err)
		os.Exit(1)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot unmarshal YAML: %v\n", filePath, err)
		os.Exit(1)
	}

	// Определяем корневой узел
	var docNode *yaml.Node
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			reportAllErrors([]error{newValidationError(0, "empty YAML document")})
		}
		docNode = root.Content[0]
	} else {
		docNode = &root
	}

	if docNode.Kind != yaml.MappingNode {
		reportAllErrors([]error{newValidationError(docNode.Line, "root must be a mapping")})
	}

	fields := getMapping(docNode)
	var errors []error

	// === Проверка обязательных полей верхнего уровня ===
	for _, field := range []string{"apiVersion", "kind", "metadata", "spec"} {
		if _, ok := fields[field]; !ok {
			errors = append(errors, fmt.Errorf("%s %s is required", filePath, field))
		}
	}
	if len(errors) > 0 {
		reportAllErrors(errors)
	}

	// apiVersion
	if n := fields["apiVersion"]; n.Kind != yaml.ScalarNode || n.Value != "v1" {
		errors = append(errors, newValidationError(n.Line, "apiVersion must be 'v1'"))
	}

	// kind
	if n := fields["kind"]; n.Kind != yaml.ScalarNode || n.Value != "Pod" {
		errors = append(errors, newValidationError(n.Line, "kind must be 'Pod'"))
	}

	// metadata
	metadata := fields["metadata"]
	if metadata.Kind != yaml.MappingNode {
		errors = append(errors, newValidationError(metadata.Line, "metadata must be a mapping"))
	} else {
		metaFields := getMapping(metadata)
		if nameNode, ok := metaFields["name"]; !ok {
			errors = append(errors, fmt.Errorf("%s metadata.name is required", filePath))
		} else if nameNode.Kind != yaml.ScalarNode || nameNode.Value == "" {
			errors = append(errors, newValidationError(nameNode.Line, "metadata.name must be non-empty string"))
		}
		// namespace и labels — не обязательны
	}

	// spec
	spec := fields["spec"]
	if spec.Kind != yaml.MappingNode {
		errors = append(errors, newValidationError(spec.Line, "spec must be a mapping"))
	} else {
		specFields := getMapping(spec)

		// os (не обязательно)
		if osNode, ok := specFields["os"]; ok {
			if osNode.Kind != yaml.MappingNode {
				errors = append(errors, newValidationError(osNode.Line, "spec.os must be a mapping"))
			} else {
				osFields := getMapping(osNode)
				if nameNode, ok := osFields["name"]; !ok {
					errors = append(errors, fmt.Errorf("%s spec.os.name is required", filePath))
				} else if nameNode.Kind != yaml.ScalarNode {
					errors = append(errors, newValidationError(nameNode.Line, "spec.os.name must be string"))
				} else if nameNode.Value != "linux" && nameNode.Value != "windows" {
					errors = append(errors, newValidationError(nameNode.Line, fmt.Sprintf("spec.os.name has unsupported value '%s'", nameNode.Value)))
				}
			}
		}

		// containers (обязательно)
		containers, ok := specFields["containers"]
		if !ok {
			errors = append(errors, fmt.Errorf("%s spec.containers is required", filePath))
		} else if containers.Kind != yaml.SequenceNode {
			errors = append(errors, newValidationError(containers.Line, "spec.containers must be a sequence"))
		} else if len(containers.Content) == 0 {
			errors = append(errors, newValidationError(containers.Line, "spec.containers must not be empty"))
		} else {
			seenNames := make(map[string]bool)
			for _, c := range containers.Content {
				if c.Kind != yaml.MappingNode {
					errors = append(errors, newValidationError(c.Line, "container must be a mapping"))
					continue
				}
				containerFields := getMapping(c)

				// name
				if nameNode, ok := containerFields["name"]; !ok {
					errors = append(errors, fmt.Errorf("%s container.name is required", filePath))
				} else if nameNode.Kind != yaml.ScalarNode {
					errors = append(errors, newValidationError(nameNode.Line, "container.name must be string"))
				} else if nameNode.Value == "" {
					errors = append(errors, newValidationError(nameNode.Line, "container.name is required"))
				} else if !snakeCaseRegex.MatchString(nameNode.Value) {
					errors = append(errors, newValidationError(nameNode.Line, fmt.Sprintf("container.name has invalid format '%s'", nameNode.Value)))
				} else if seenNames[nameNode.Value] {
					errors = append(errors, newValidationError(nameNode.Line, fmt.Sprintf("container.name '%s' is not unique", nameNode.Value)))
				} else {
					seenNames[nameNode.Value] = true
				}

				// image
				if imageNode, ok := containerFields["image"]; !ok {
					errors = append(errors, fmt.Errorf("%s container.image is required", filePath))
				} else if imageNode.Kind != yaml.ScalarNode {
					errors = append(errors, newValidationError(imageNode.Line, "container.image must be string"))
				} else {
					image := imageNode.Value
					if !strings.HasPrefix(image, "registry.bigbrother.io/") {
						errors = append(errors, newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
					} else {
						parts := strings.Split(image, ":")
						if len(parts) < 2 {
							errors = append(errors, newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
						} else {
							tag := parts[len(parts)-1]
							if tag == "" {
								errors = append(errors, newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
							}
						}
					}
				}

				// ports
				if portsNode, ok := containerFields["ports"]; ok {
					if portsNode.Kind != yaml.SequenceNode {
						errors = append(errors, newValidationError(portsNode.Line, "container.ports must be a sequence"))
					} else {
						for _, p := range portsNode.Content {
							if p.Kind != yaml.MappingNode {
								errors = append(errors, newValidationError(p.Line, "container port must be a mapping"))
								continue
							}
							portFields := getMapping(p)
							if cpNode, ok := portFields["containerPort"]; !ok {
								errors = append(errors, fmt.Errorf("%s containerPort is required", filePath))
							} else if cpNode.Kind != yaml.ScalarNode {
								errors = append(errors, newValidationError(cpNode.Line, "containerPort must be int"))
							} else {
								port, err := strconv.Atoi(cpNode.Value)
								if err != nil {
									errors = append(errors, newValidationError(cpNode.Line, "containerPort must be int"))
								} else if port <= 0 || port >= 65536 {
									errors = append(errors, newValidationError(cpNode.Line, "containerPort value out of range"))
								}
							}
							if protoNode, ok := portFields["protocol"]; ok {
								if protoNode.Kind != yaml.ScalarNode {
									errors = append(errors, newValidationError(protoNode.Line, "protocol must be string"))
								} else if protoNode.Value != "TCP" && protoNode.Value != "UDP" {
									errors = append(errors, newValidationError(protoNode.Line, fmt.Sprintf("protocol has unsupported value '%s'", protoNode.Value)))
								}
							}
						}
					}
				}

				// readinessProbe & livenessProbe
				for _, probeName := range []string{"readinessProbe", "livenessProbe"} {
					if probeNode, ok := containerFields[probeName]; ok {
						if probeNode.Kind != yaml.MappingNode {
							errors = append(errors, newValidationError(probeNode.Line, probeName+" must be a mapping"))
						} else {
							probeFields := getMapping(probeNode)
							if httpGetNode, ok := probeFields["httpGet"]; !ok {
								errors = append(errors, fmt.Errorf("%s %s.httpGet is required", filePath, probeName))
							} else if httpGetNode.Kind != yaml.MappingNode {
								errors = append(errors, newValidationError(httpGetNode.Line, probeName+".httpGet must be a mapping"))
							} else {
								httpFields := getMapping(httpGetNode)
								// path
								if pathNode, ok := httpFields["path"]; !ok {
									errors = append(errors, fmt.Errorf("%s %s.httpGet.path is required", filePath, probeName))
								} else if pathNode.Kind != yaml.ScalarNode || !strings.HasPrefix(pathNode.Value, "/") {
									errors = append(errors, newValidationError(pathNode.Line, probeName+".httpGet.path must be absolute"))
								}
								// port
								if portNode, ok := httpFields["port"]; !ok {
									errors = append(errors, fmt.Errorf("%s %s.httpGet.port is required", filePath, probeName))
								} else if portNode.Kind != yaml.ScalarNode {
									errors = append(errors, newValidationError(portNode.Line, probeName+".httpGet.port must be int"))
								} else {
									port, err := strconv.Atoi(portNode.Value)
									if err != nil || port <= 0 || port >= 65536 {
										errors = append(errors, newValidationError(portNode.Line, probeName+".httpGet.port value out of range"))
									}
								}
							}
						}
					}
				}

				// resources (обязательно)
				if resNode, ok := containerFields["resources"]; !ok {
					errors = append(errors, fmt.Errorf("%s container.resources is required", filePath))
				} else if resNode.Kind != yaml.MappingNode {
					errors = append(errors, newValidationError(resNode.Line, "container.resources must be a mapping"))
				} else {
					resFields := getMapping(resNode)
					for _, section := range []string{"requests", "limits"} {
						if secNode, ok := resFields[section]; ok {
							if secNode.Kind != yaml.MappingNode {
								errors = append(errors, newValidationError(secNode.Line, "resources."+section+" must be a mapping"))
							} else {
								secFields := getMapping(secNode)
								if cpuNode, ok := secFields["cpu"]; ok {
									if cpuNode.Kind != yaml.ScalarNode {
										errors = append(errors, newValidationError(cpuNode.Line, "resources."+section+".cpu must be int"))
									} else if _, err := strconv.Atoi(cpuNode.Value); err != nil {
										errors = append(errors, newValidationError(cpuNode.Line, "resources."+section+".cpu must be int"))
									}
								}
								if memNode, ok := secFields["memory"]; ok {
									if memNode.Kind != yaml.ScalarNode || !memoryRegex.MatchString(memNode.Value) {
										errors = append(errors, newValidationError(memNode.Line, fmt.Sprintf("resources.%s.memory has invalid format '%s'", section, memNode.Value)))
									}
								}
							}
						}
					}
				}
			}
		}
	}

	reportAllErrors(errors)
}
