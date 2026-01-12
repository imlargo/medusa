package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/imlargo/medusa/cmd/medusa/templates"
	"github.com/imlargo/medusa/cmd/medusa/utils"
	"github.com/spf13/cobra"
)

var (
	newTemplate    string
	newModule      string
	newPath        string
	newNoGit       bool
	newNoInstall   bool
	newInteractive bool
)

var newCmd = &cobra.Command{
	Use:   "new [project-name]",
	Short: "Create a new Medusa project",
	Long:  `Create a new Medusa project with the specified template and configuration.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

func init() {
	newCmd.Flags().StringVarP(&newTemplate, "template", "t", "full", "Project template (full, minimal, microservice)")
	newCmd.Flags().StringVarP(&newModule, "module", "m", "", "Go module name")
	newCmd.Flags().StringVarP(&newPath, "path", "p", ".", "Directory to create project in")
	newCmd.Flags().BoolVar(&newNoGit, "no-git", false, "Skip git initialization")
	newCmd.Flags().BoolVar(&newNoInstall, "no-install", false, "Skip go mod download")
	newCmd.Flags().BoolVarP(&newInteractive, "interactive", "i", false, "Interactive mode")
}

func runNew(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	utils.PrintBanner()
	utils.PrintSuccess("Creating new Medusa project.. .\n")

	// Validate project name
	if !utils.IsValidProjectName(projectName) {
		return fmt.Errorf("invalid project name: %s", projectName)
	}

	// Determine module name
	if newModule == "" {
		newModule = projectName
		if !strings.Contains(newModule, "/") {
			newModule = "github.com/user/" + projectName
		}
	}

	// Create project directory
	projectPath := filepath.Join(newPath, projectName)
	if err := createProjectStructure(projectPath, projectName, newModule, newTemplate); err != nil {
		return fmt.Errorf("failed to create project structure: %w", err)
	}

	// Initialize git
	if !newNoGit {
		utils.PrintStep("Initializing git repository")
		if err := initGit(projectPath); err != nil {
			utils.PrintWarning("Failed to initialize git: " + err.Error())
		} else {
			utils.PrintCheckmark("Git initialized")
		}
	}

	// Install dependencies
	if !newNoInstall {
		utils.PrintStep("Installing dependencies")
		if err := installDependencies(projectPath); err != nil {
			utils.PrintWarning("Failed to install dependencies: " + err.Error())
		} else {
			utils.PrintCheckmark("Dependencies installed")
		}
	}

	// Print success message
	printSuccessMessage(projectName, projectPath, newModule)

	return nil
}

func createProjectStructure(projectPath, projectName, moduleName, template string) error {
	// Create directories
	utils.PrintStep("Creating directory structure")
	dirs := []string{
		"cmd/api",
		"internal/config",
		"internal/database",
		"internal/handlers",
		"internal/models",
		"internal/repository",
		"internal/service",
		"internal/store",
		"pkg",
		"migrations",
		"tests",
	}

	if template == "full" {
		dirs = append(dirs, "cmd/worker", "scripts", "docs")
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(projectPath, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return err
		}
	}
	utils.PrintCheckmark("Directory structure created")

	// Generate files
	utils.PrintStep("Generating files")

	// go.mod
	if err := utils.WriteFile(
		filepath.Join(projectPath, "go.mod"),
		templates.GenerateGoMod(moduleName),
	); err != nil {
		return err
	}

	// main.go
	if err := utils.WriteFile(
		filepath.Join(projectPath, "cmd/api/main.go"),
		templates.GenerateMainFile(moduleName, projectName),
	); err != nil {
		return err
	}

	// . env. example
	if err := utils.WriteFile(
		filepath.Join(projectPath, ".env.example"),
		templates.GenerateEnvExample(),
	); err != nil {
		return err
	}

	// . gitignore
	if err := utils.WriteFile(
		filepath.Join(projectPath, ".gitignore"),
		templates.GenerateGitignore(),
	); err != nil {
		return err
	}

	// README. md
	if err := utils.WriteFile(
		filepath.Join(projectPath, "README.md"),
		templates.GenerateReadme(projectName, moduleName),
	); err != nil {
		return err
	}

	// Makefile
	if err := utils.WriteFile(
		filepath.Join(projectPath, "Makefile"),
		templates.GenerateMakefile(projectName),
	); err != nil {
		return err
	}

	// docker-compose.yml
	if template == "full" {
		if err := utils.WriteFile(
			filepath.Join(projectPath, "docker-compose.yml"),
			templates.GenerateDockerCompose(projectName),
		); err != nil {
			return err
		}
	}

	// Dockerfile
	if err := utils.WriteFile(
		filepath.Join(projectPath, "Dockerfile"),
		templates.GenerateDockerfile(),
	); err != nil {
		return err
	}

	// config.go
	if err := utils.WriteFile(
		filepath.Join(projectPath, "internal/config/config.go"),
		templates.GenerateConfigFile(),
	); err != nil {
		return err
	}

	utils.PrintCheckmark("Files generated")

	return nil
}

func initGit(projectPath string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = projectPath
	return cmd.Run()
}

func installDependencies(projectPath string) error {
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = projectPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printSuccessMessage(projectName, projectPath, moduleName string) {
	utils.PrintSuccess("\n✨ Project created successfully!\n")

	fmt.Println()
	utils.PrintInfo("📁 Project:   " + projectName)
	utils.PrintInfo("📦 Module:   " + moduleName)
	utils.PrintInfo("📂 Path:     " + projectPath)

	fmt.Println()
	utils.PrintBold("Next steps:")
	fmt.Println("  cd " + projectName)
	fmt.Println("  cp .env.example .env")
	fmt.Println("  # Edit .env with your configuration")
	fmt.Println("  docker-compose up -d  # Start PostgreSQL and Redis")
	fmt.Println("  make run              # Start the server")

	fmt.Println()
	utils.PrintInfo("📚 Documentation:  https://github.com/imlargo/medusa")
	fmt.Println()
}
