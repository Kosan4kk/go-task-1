package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type PodSpec struct {
	Containers []Container `yaml:"containers"`
}

type Container struct {
	Name      string                 `yaml:"name"`
	Image     string                 `yaml:"image"`
	Resources map[string]interface{} `yaml:"resources"`
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

	var pod Pod
	if err := yaml.Unmarshal(content, &pod); err != nil {
		return fmt.Errorf("%s: cannot unmarshal YAML: %v", filename, err)
	}

	// Проверка apiVersion
	if pod.APIVersion != "v1" {
		return fmt.Errorf("apiVersion is required and must be 'v1'")
	}

	// Проверка kind
	if pod.Kind != "Pod" {
		return fmt.Errorf("kind is required and must be 'Pod'")
	}

	// Проверка metadata.name
	if pod.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	// Проверка containers
	if len(pod.Spec.Containers) == 0 {
		return fmt.Errorf("spec.containers is required and must contain at least one container")
	}

	// Проверка каждого контейнера
	for i, container := range pod.Spec.Containers {
		if container.Name == "" {
			return fmt.Errorf("container %d name is required", i)
		}
		if container.Image == "" {
			return fmt.Errorf("container %d image is required", i)
		}
		if container.Resources == nil {
			return fmt.Errorf("container %d resources is required", i)
		}
	}

	return nil
}
