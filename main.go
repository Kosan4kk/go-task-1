package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError представляет ошибку валидации с номером строки
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

// helper to create validation error
func newValidationError(line int, msg string) error {
	return ValidationError{Line: line, Msg: msg}
}

// helper to report error and exit
func reportError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// Проверка snake_case
var snakeCaseRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Проверка формата памяти: "123Mi", "2Gi" и т.д.
var memoryRegex = regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: yamlvalid <file.yaml>")
		os.Exit(1)
	}

	filePath := os.Args[1]
	content, err := os.ReadFile(filePath)
	if err != nil {
		reportError(fmt.Errorf("%s: cannot read file: %w", filePath, err))
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		reportError(fmt.Errorf("%s: cannot unmarshal YAML: %w", filePath, err))
	}

	if len(root.Content) == 0 {
		reportError(newValidationError(0, "empty YAML document"))
	}

	doc := root.Content[0]
	if doc.Kind != yaml.DocumentNode {
		doc = &root // если нет явного документа
	}

	if len(doc.Content) == 0 {
		reportError(newValidationError(0, "no content in YAML document"))
	}

	topLevel := doc.Content[0]
	if topLevel.Kind != yaml.MappingNode {
		reportError(newValidationError(topLevel.Line, "root must be a mapping"))
	}

	// Собираем поля верхнего уровня
	fields := map[string]*yaml.Node{}
	for i := 0; i < len(topLevel.Content); i += 2 {
		keyNode := topLevel.Content[i]
		valueNode := topLevel.Content[i+1]
		fields[keyNode.Value] = valueNode
	}

	// === Проверка обязательных полей верхнего уровня ===
	requiredTopFields := []string{"apiVersion", "kind", "metadata", "spec"}
	for _, field := range requiredTopFields {
		if _, ok := fields[field]; !ok {
			reportError(fmt.Errorf("%s %s is required", filePath, field))
		}
	}

	// apiVersion
	if apiVersion := fields["apiVersion"]; apiVersion.Kind != yaml.ScalarNode || apiVersion.Value != "v1" {
		reportError(newValidationError(apiVersion.Line, "apiVersion must be 'v1'"))
	}

	// kind
	if kind := fields["kind"]; kind.Kind != yaml.ScalarNode || kind.Value != "Pod" {
		reportError(newValidationError(kind.Line, "kind must be 'Pod'"))
	}

	// metadata
	metadata := fields["metadata"]
	if metadata.Kind != yaml.MappingNode {
		reportError(newValidationError(metadata.Line, "metadata must be a mapping"))
	}
	metadataFields := getMapping(metadata)
	if _, ok := metadataFields["name"]; !ok {
		reportError(fmt.Errorf("%s metadata.name is required", filePath))
	}
	if nameNode := metadataFields["name"]; nameNode.Kind != yaml.ScalarNode {
		reportError(newValidationError(nameNode.Line, "metadata.name must be string"))
	}

	// namespace и labels не обязательны — пропускаем, если есть

	// spec
	spec := fields["spec"]
	if spec.Kind != yaml.MappingNode {
		reportError(newValidationError(spec.Line, "spec must be a mapping"))
	}
	specFields := getMapping(spec)

	// os (не обязательно)
	if osNode, ok := specFields["os"]; ok {
		if osNode.Kind != yaml.MappingNode {
			reportError(newValidationError(osNode.Line, "spec.os must be a mapping"))
		}
		osFields := getMapping(osNode)
		if nameNode, ok := osFields["name"]; ok {
			if nameNode.Kind != yaml.ScalarNode {
				reportError(newValidationError(nameNode.Line, "spec.os.name must be string"))
			}
			if nameNode.Value != "linux" && nameNode.Value != "windows" {
				reportError(newValidationError(nameNode.Line, fmt.Sprintf("spec.os.name has unsupported value '%s'", nameNode.Value)))
			}
		} else {
			reportError(fmt.Errorf("%s spec.os.name is required", filePath))
		}
	}

	// containers (обязательно)
	containers, ok := specFields["containers"]
	if !ok {
		reportError(fmt.Errorf("%s spec.containers is required", filePath))
	}
	if containers.Kind != yaml.SequenceNode {
		reportError(newValidationError(containers.Line, "spec.containers must be a sequence"))
	}

	if len(containers.Content) == 0 {
		reportError(newValidationError(containers.Line, "spec.containers must not be empty"))
	}

	containerNames := make(map[string]bool)
	for _, container := range containers.Content {
		if container.Kind != yaml.MappingNode {
			reportError(newValidationError(container.Line, "container must be a mapping"))
		}
		containerFields := getMapping(container)

		// name (обязательно, snake_case)
		nameNode, ok := containerFields["name"]
		if !ok {
			reportError(fmt.Errorf("%s container.name is required", filePath))
		}
		if nameNode.Kind != yaml.ScalarNode {
			reportError(newValidationError(nameNode.Line, "container.name must be string"))
		}
		if !snakeCaseRegex.MatchString(nameNode.Value) {
			reportError(newValidationError(nameNode.Line, fmt.Sprintf("container.name has invalid format '%s'", nameNode.Value)))
		}
		if containerNames[nameNode.Value] {
			reportError(newValidationError(nameNode.Line, fmt.Sprintf("container.name '%s' is not unique", nameNode.Value)))
		}
		containerNames[nameNode.Value] = true

		// image (обязательно, registry.bigbrother.io/..., с тегом)
		imageNode, ok := containerFields["image"]
		if !ok {
			reportError(fmt.Errorf("%s container.image is required", filePath))
		}
		if imageNode.Kind != yaml.ScalarNode {
			reportError(newValidationError(imageNode.Line, "container.image must be string"))
		}
		image := imageNode.Value
		parts := strings.Split(image, "/")
		if len(parts) < 2 {
			reportError(newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
		}
		if parts[0] != "registry.bigbrother.io" {
			reportError(newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
		}
		tagParts := strings.Split(parts[len(parts)-1], ":")
		if len(tagParts) < 2 {
			reportError(newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
		}
		if tagParts[len(tagParts)-1] == "" {
			reportError(newValidationError(imageNode.Line, fmt.Sprintf("container.image has invalid format '%s'", image)))
		}

		// ports (не обязательно)
		if portsNode, ok := containerFields["ports"]; ok {
			if portsNode.Kind != yaml.SequenceNode {
				reportError(newValidationError(portsNode.Line, "container.ports must be a sequence"))
			}
			for _, portNode := range portsNode.Content {
				if portNode.Kind != yaml.MappingNode {
					reportError(newValidationError(portNode.Line, "container port must be a mapping"))
				}
				portFields := getMapping(portNode)
				containerPortNode, ok := portFields["containerPort"]
				if !ok {
					reportError(fmt.Errorf("%s containerPort is required", filePath))
				}
				if containerPortNode.Kind != yaml.ScalarNode {
					reportError(newValidationError(containerPortNode.Line, "containerPort must be int"))
				}
				port, err := strconv.Atoi(containerPortNode.Value)
				if err != nil {
					reportError(newValidationError(containerPortNode.Line, "containerPort must be int"))
				}
				if port <= 0 || port >= 65536 {
					reportError(newValidationError(containerPortNode.Line, "containerPort value out of range"))
				}

				// protocol (не обязательно, по умолчанию TCP)
				if protoNode, ok := portFields["protocol"]; ok {
					if protoNode.Kind != yaml.ScalarNode {
						reportError(newValidationError(protoNode.Line, "protocol must be string"))
					}
					if protoNode.Value != "TCP" && protoNode.Value != "UDP" {
						reportError(newValidationError(protoNode.Line, fmt.Sprintf("protocol has unsupported value '%s'", protoNode.Value)))
					}
				}
			}
		}

		// readinessProbe и livenessProbe (не обязательно, но если есть — httpGet обязателен)
		for _, probeName := range []string{"readinessProbe", "livenessProbe"} {
			if probeNode, ok := containerFields[probeName]; ok {
				if probeNode.Kind != yaml.MappingNode {
					reportError(newValidationError(probeNode.Line, fmt.Sprintf("%s must be a mapping", probeName)))
				}
				probeFields := getMapping(probeNode)
				httpGetNode, ok := probeFields["httpGet"]
				if !ok {
					reportError(fmt.Errorf("%s %s.httpGet is required", filePath, probeName))
				}
				if httpGetNode.Kind != yaml.MappingNode {
					reportError(newValidationError(httpGetNode.Line, fmt.Sprintf("%s.httpGet must be a mapping", probeName)))
				}
				httpFields := getMapping(httpGetNode)
				// path
				pathNode, ok := httpFields["path"]
				if !ok {
					reportError(fmt.Errorf("%s %s.httpGet.path is required", filePath, probeName))
				}
				if pathNode.Kind != yaml.ScalarNode {
					reportError(newValidationError(pathNode.Line, fmt.Sprintf("%s.httpGet.path must be string", probeName)))
				}
				if !strings.HasPrefix(pathNode.Value, "/") {
					reportError(newValidationError(pathNode.Line, fmt.Sprintf("%s.httpGet.path must be absolute", probeName)))
				}
				// port
				portNode, ok := httpFields["port"]
				if !ok {
					reportError(fmt.Errorf("%s %s.httpGet.port is required", filePath, probeName))
				}
				if portNode.Kind != yaml.ScalarNode {
					reportError(newValidationError(portNode.Line, fmt.Sprintf("%s.httpGet.port must be int", probeName)))
				}
				port, err := strconv.Atoi(portNode.Value)
				if err != nil {
					reportError(newValidationError(portNode.Line, fmt.Sprintf("%s.httpGet.port must be int", probeName)))
				}
				if port <= 0 || port >= 65536 {
					reportError(newValidationError(portNode.Line, fmt.Sprintf("%s.httpGet.port value out of range", probeName)))
				}
			}
		}

		// resources (обязательно)
		resourcesNode, ok := containerFields["resources"]
		if !ok {
			reportError(fmt.Errorf("%s container.resources is required", filePath))
		}
		if resourcesNode.Kind != yaml.MappingNode {
			reportError(newValidationError(resourcesNode.Line, "container.resources must be a mapping"))
		}
		resourcesFields := getMapping(resourcesNode)

		// Проверяем requests и limits (не обязательны, но если есть — проверяем формат)
		for _, resType := range []string{"requests", "limits"} {
			if resNode, ok := resourcesFields[resType]; ok {
				if resNode.Kind != yaml.MappingNode {
					reportError(newValidationError(resNode.Line, fmt.Sprintf("resources.%s must be a mapping", resType)))
				}
				resMap := getMapping(resNode)
				for k, v := range resMap {
					if v.Kind != yaml.ScalarNode {
						reportError(newValidationError(v.Line, fmt.Sprintf("resources.%s.%s must be scalar", resType, k)))
					}
					switch k {
					case "cpu":
						if _, err := strconv.Atoi(v.Value); err != nil {
							reportError(newValidationError(v.Line, fmt.Sprintf("resources.%s.cpu must be integer", resType)))
						}
					case "memory":
						if !memoryRegex.MatchString(v.Value) {
							reportError(newValidationError(v.Line, fmt.Sprintf("resources.%s.memory has invalid format '%s'", resType, v.Value)))
						}
					default:
						// Неизвестный ресурс — игнорируем (или можно запретить)
					}
				}
			}
		}
	}

	// Всё прошло — успех
	os.Exit(0)
}

// getMapping преобразует yaml.MappingNode в map[string]*yaml.Node
func getMapping(node *yaml.Node) map[string]*yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	result := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		result[key.Value] = value
	}
	return result
}
