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

	// ДЕБАГ: сразу выведем что нашли в os
	doc := node.Content[0]
	spec := findNode(doc, "spec")
	if spec != nil {
		osNode := findNode(spec, "os")
		if osNode != nil {
			fmt.Fprintf(os.Stderr, "%s:%d os has unsupported value '%s'\n", filename, osNode.Line, osNode.Value)
			os.Exit(1)
		}
	}

	// Если дошли сюда - ошибок нет
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
