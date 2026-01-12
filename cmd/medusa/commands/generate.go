// cmd/medusa/commands/generate.go
package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/imlargo/medusa/cmd/medusa/templates"
	"github.com/imlargo/medusa/cmd/medusa/utils"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"g"},
	Short:   "Generate code (handler, model, repository, etc.)",
	Long:    `Generate boilerplate code for handlers, models, repositories, services, and more.`,
}

var generateHandlerCmd = &cobra.Command{
	Use:     "handler [name]",
	Aliases: []string{"h"},
	Short:   "Generate HTTP handler",
	Args:    cobra.ExactArgs(1),
	RunE:    runGenerateHandler,
}

var generateModelCmd = &cobra.Command{
	Use:     "model [name]",
	Aliases: []string{"m"},
	Short:   "Generate model/entity",
	Args:    cobra.ExactArgs(1),
	RunE:    runGenerateModel,
}

var generateRepositoryCmd = &cobra.Command{
	Use:     "repository [name]",
	Aliases: []string{"repo", "r"},
	Short:   "Generate repository",
	Args:    cobra.ExactArgs(1),
	RunE:    runGenerateRepository,
}

var generateServiceCmd = &cobra.Command{
	Use:     "service [name]",
	Aliases: []string{"s"},
	Short:   "Generate service",
	Args:    cobra.ExactArgs(1),
	RunE:    runGenerateService,
}

var generateCrudCmd = &cobra.Command{
	Use:   "crud [name]",
	Short: "Generate complete CRUD (model + repository + service + handler)",
	Args:  cobra.ExactArgs(1),
	RunE:  runGenerateCrud,
}

var (
	genMethods    []string
	genFields     []string
	genResource   bool
	genAuth       bool
	genValidate   bool
	genPath       string
	genForce      bool
	genTimestamps bool
	genSoftDelete bool
	genTable      string
	genPagination bool
	genMigration  bool
)

func init() {
	// Handler flags
	generateHandlerCmd.Flags().StringSliceVar(&genMethods, "methods", []string{}, "HTTP methods (get,post,put,delete)")
	generateHandlerCmd.Flags().BoolVar(&genResource, "resource", false, "Generate RESTful resource handler")
	generateHandlerCmd.Flags().BoolVar(&genAuth, "auth", false, "Add authentication")
	generateHandlerCmd.Flags().BoolVar(&genValidate, "validate", false, "Add validation")
	generateHandlerCmd.Flags().StringVar(&genPath, "path", "internal/handlers", "Output directory")
	generateHandlerCmd.Flags().BoolVarP(&genForce, "force", "f", false, "Overwrite existing files")

	// Model flags
	generateModelCmd.Flags().StringSliceVar(&genFields, "fields", []string{}, "Model fields (name: type: tags)")
	generateModelCmd.Flags().BoolVar(&genTimestamps, "timestamps", true, "Add CreatedAt/UpdatedAt")
	generateModelCmd.Flags().BoolVar(&genSoftDelete, "soft-delete", false, "Add soft delete")
	generateModelCmd.Flags().StringVar(&genTable, "table", "", "Custom table name")
	generateModelCmd.Flags().StringVar(&genPath, "path", "internal/models", "Output directory")
	generateModelCmd.Flags().BoolVarP(&genForce, "force", "f", false, "Overwrite existing files")

	// Repository flags
	generateRepositoryCmd.Flags().BoolVar(&genPagination, "pagination", false, "Add pagination support")
	generateRepositoryCmd.Flags().StringVar(&genPath, "path", "internal/repository", "Output directory")
	generateRepositoryCmd.Flags().BoolVarP(&genForce, "force", "f", false, "Overwrite existing files")

	// Service flags
	generateServiceCmd.Flags().StringVar(&genPath, "path", "internal/service", "Output directory")
	generateServiceCmd.Flags().BoolVarP(&genForce, "force", "f", false, "Overwrite existing files")

	// CRUD flags
	generateCrudCmd.Flags().StringSliceVar(&genFields, "fields", []string{}, "Model fields")
	generateCrudCmd.Flags().BoolVar(&genAuth, "auth", false, "Add authentication")
	generateCrudCmd.Flags().BoolVar(&genSoftDelete, "soft-delete", false, "Enable soft delete")
	generateCrudCmd.Flags().BoolVar(&genPagination, "pagination", true, "Add pagination")
	generateCrudCmd.Flags().BoolVar(&genMigration, "migration", false, "Generate migration")

	// Add subcommands
	generateCmd.AddCommand(generateHandlerCmd)
	generateCmd.AddCommand(generateModelCmd)
	generateCmd.AddCommand(generateRepositoryCmd)
	generateCmd.AddCommand(generateServiceCmd)
	generateCmd.AddCommand(generateCrudCmd)
}

func runGenerateHandler(cmd *cobra.Command, args []string) error {
	name := args[0]

	utils.PrintStep(fmt.Sprintf("Generating handler:  %s", name))

	// Parse name (support nested paths like admin/user)
	parts := strings.Split(name, "/")
	handlerName := parts[len(parts)-1]

	// Capitalize
	structName := utils.Capitalize(handlerName)

	// If resource flag, generate RESTful methods
	if genResource {
		genMethods = []string{"get", "post", "put", "delete", "list"}
	}

	// Generate handler file
	handlerCode := templates.GenerateHandler(structName, genMethods, genAuth, genResource)
	handlerFile := filepath.Join(genPath, utils.ToSnakeCase(handlerName)+"_handler.go")

	if err := utils.WriteFile(handlerFile, handlerCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", handlerFile))

	// Generate test file
	testCode := templates.GenerateHandlerTest(structName)
	testFile := filepath.Join(genPath, utils.ToSnakeCase(handlerName)+"_handler_test.go")

	if err := utils.WriteFile(testFile, testCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", testFile))

	// Print routes
	if genResource {
		utils.PrintSuccess("\n📄 Generated routes:")
		fmt.Printf("  - Create%s (POST /%s)\n", structName, utils.ToSnakeCase(handlerName)+"s")
		fmt.Printf("  - Get%s (GET /%s/: id)\n", structName, utils.ToSnakeCase(handlerName)+"s")
		fmt.Printf("  - List%s (GET /%s)\n", structName, utils.ToSnakeCase(handlerName)+"s")
		fmt.Printf("  - Update%s (PUT /%s/:id)\n", structName, utils.ToSnakeCase(handlerName)+"s")
		fmt.Printf("  - Delete%s (DELETE /%s/:id)\n", structName, utils.ToSnakeCase(handlerName)+"s")
	}

	utils.PrintInfo("\nDon't forget to register routes in cmd/api/main.go!")

	return nil
}

func runGenerateModel(cmd *cobra.Command, args []string) error {
	name := args[0]

	utils.PrintStep(fmt.Sprintf("Generating model: %s", name))

	structName := utils.Capitalize(name)

	// Parse fields
	fields := parseFields(genFields)

	// Determine table name
	tableName := genTable
	if tableName == "" {
		tableName = utils.ToSnakeCase(utils.Pluralize(name))
	}

	// Generate model file
	modelCode := templates.GenerateModel(structName, tableName, fields, genTimestamps, genSoftDelete)
	modelFile := filepath.Join(genPath, utils.ToSnakeCase(name)+".go")

	if err := utils.WriteFile(modelFile, modelCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", modelFile))

	utils.PrintSuccess("\n📄 Model:  " + structName)
	utils.PrintInfo(fmt.Sprintf("📋 Fields: %d + timestamps", len(fields)))
	utils.PrintInfo(fmt.Sprintf("🗃️  Table: %s", tableName))

	return nil
}

func runGenerateRepository(cmd *cobra.Command, args []string) error {
	name := args[0]

	utils.PrintStep(fmt.Sprintf("Generating repository: %s", name))

	structName := utils.Capitalize(name)

	// Generate repository file
	repoCode := templates.GenerateRepository(structName, genPagination)
	repoFile := filepath.Join(genPath, utils.ToSnakeCase(name)+"_repository.go")

	if err := utils.WriteFile(repoFile, repoCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", repoFile))

	// Generate test file
	testCode := templates.GenerateRepositoryTest(structName)
	testFile := filepath.Join(genPath, utils.ToSnakeCase(name)+"_repository_test.go")

	if err := utils.WriteFile(testFile, testCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", testFile))

	methods := "Create, FindByID, FindAll, Update, Delete"
	if genPagination {
		methods += ", Paginate"
	}

	utils.PrintSuccess("\n📄 Repository: " + structName + "Repository")
	utils.PrintInfo("🔧 Methods: " + methods)

	return nil
}

func runGenerateService(cmd *cobra.Command, args []string) error {
	name := args[0]

	utils.PrintStep(fmt.Sprintf("Generating service: %s", name))

	structName := utils.Capitalize(name)

	// Generate service file
	serviceCode := templates.GenerateService(structName)
	serviceFile := filepath.Join(genPath, utils.ToSnakeCase(name)+"_service.go")

	if err := utils.WriteFile(serviceFile, serviceCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", serviceFile))

	// Generate test file
	testCode := templates.GenerateServiceTest(structName)
	testFile := filepath.Join(genPath, utils.ToSnakeCase(name)+"_service_test.go")

	if err := utils.WriteFile(testFile, testCode); err != nil {
		return err
	}

	utils.PrintCheckmark(fmt.Sprintf("Created %s", testFile))

	utils.PrintSuccess("\n📄 Service: " + structName + "Service")

	return nil
}

func runGenerateCrud(cmd *cobra.Command, args []string) error {
	name := args[0]

	utils.PrintBanner()
	utils.PrintSuccess(fmt.Sprintf("Generating CRUD for %s.. .\n", utils.Capitalize(name)))

	// Generate Model
	if err := generateModel(name); err != nil {
		return err
	}

	// Generate Repository
	if err := generateRepository(name); err != nil {
		return err
	}

	// Generate Service
	if err := generateService(name); err != nil {
		return err
	}

	// Generate Handler
	genResource = true
	if err := generateHandler(name); err != nil {
		return err
	}

	// Generate Migration
	if genMigration {
		utils.PrintStep("Generating migration")
		// TODO: Generate migration
		utils.PrintWarning("Migration generation not implemented yet")
	}

	// Print summary
	printCrudSummary(name)

	return nil
}

func generateModel(name string) error {
	utils.PrintStep("Generating model")
	fields := parseFields(genFields)
	structName := utils.Capitalize(name)
	tableName := utils.ToSnakeCase(utils.Pluralize(name))

	modelCode := templates.GenerateModel(structName, tableName, fields, genTimestamps, genSoftDelete)
	modelFile := filepath.Join("internal/models", utils.ToSnakeCase(name)+".go")

	if err := utils.WriteFile(modelFile, modelCode); err != nil {
		return err
	}

	utils.PrintCheckmark("Created " + modelFile)
	return nil
}

func generateRepository(name string) error {
	utils.PrintStep("Generating repository")
	structName := utils.Capitalize(name)

	repoCode := templates.GenerateRepository(structName, genPagination)
	repoFile := filepath.Join("internal/repository", utils.ToSnakeCase(name)+"_repository.go")

	if err := utils.WriteFile(repoFile, repoCode); err != nil {
		return err
	}

	utils.PrintCheckmark("Created " + repoFile)

	// Test file
	testCode := templates.GenerateRepositoryTest(structName)
	testFile := filepath.Join("internal/repository", utils.ToSnakeCase(name)+"_repository_test.go")
	utils.WriteFile(testFile, testCode)
	utils.PrintCheckmark("Created " + testFile)

	return nil
}

func generateService(name string) error {
	utils.PrintStep("Generating service")
	structName := utils.Capitalize(name)

	serviceCode := templates.GenerateService(structName)
	serviceFile := filepath.Join("internal/service", utils.ToSnakeCase(name)+"_service.go")

	if err := utils.WriteFile(serviceFile, serviceCode); err != nil {
		return err
	}

	utils.PrintCheckmark("Created " + serviceFile)

	// Test file
	testCode := templates.GenerateServiceTest(structName)
	testFile := filepath.Join("internal/service", utils.ToSnakeCase(name)+"_service_test.go")
	utils.WriteFile(testFile, testCode)
	utils.PrintCheckmark("Created " + testFile)

	return nil
}

func generateHandler(name string) error {
	utils.PrintStep("Generating handler")
	structName := utils.Capitalize(name)

	handlerCode := templates.GenerateHandler(structName, nil, genAuth, true)
	handlerFile := filepath.Join("internal/handlers", utils.ToSnakeCase(name)+"_handler.go")

	if err := utils.WriteFile(handlerFile, handlerCode); err != nil {
		return err
	}

	utils.PrintCheckmark("Created " + handlerFile)

	// Test file
	testCode := templates.GenerateHandlerTest(structName)
	testFile := filepath.Join("internal/handlers", utils.ToSnakeCase(name)+"_handler_test.go")
	utils.WriteFile(testFile, testCode)
	utils.PrintCheckmark("Created " + testFile)

	return nil
}

func printCrudSummary(name string) {
	structName := utils.Capitalize(name)
	snakeName := utils.ToSnakeCase(name)

	utils.PrintSuccess("\n📋 Summary:")
	fmt.Printf("  Model:        internal/models/%s.go\n", snakeName)
	fmt.Printf("  Repository:  internal/repository/%s_repository.go\n", snakeName)
	fmt.Printf("  Service:     internal/service/%s_service.go\n", snakeName)
	fmt.Printf("  Handler:     internal/handlers/%s_handler.go\n", snakeName)

	fmt.Println()
	utils.PrintBold("🔗 Register routes in cmd/api/main. go:")
	fmt.Printf("  %sHandler := handlers. New%sHandler(... )\n", snakeName, structName)
	fmt.Println("  api := router.Group(\"/api/v1\")")
	if genAuth {
		fmt.Println("  api.Use(middleware.AuthTokenMiddleware(jwtAuth))")
	}
	fmt.Println("  {")
	fmt.Printf("      api.POST(\"/%ss\", %sHandler.Create%s)\n", snakeName, snakeName, structName)
	fmt.Printf("      api.GET(\"/%ss\", %sHandler.List%s)\n", snakeName, snakeName, structName)
	fmt.Printf("      api.GET(\"/%ss/:id\", %sHandler.Get%s)\n", snakeName, snakeName, structName)
	fmt.Printf("      api. PUT(\"/%ss/:id\", %sHandler.Update%s)\n", snakeName, snakeName, structName)
	fmt.Printf("      api.DELETE(\"/%ss/:id\", %sHandler.Delete%s)\n", snakeName, snakeName, structName)
	fmt.Println("  }")

	if genMigration {
		fmt.Println()
		utils.PrintInfo("🚀 Run migration:  medusa migrate up")
	}
}

func parseFields(fieldStrings []string) []templates.Field {
	var fields []templates.Field

	for _, fieldStr := range fieldStrings {
		parts := strings.Split(fieldStr, ":")
		if len(parts) < 2 {
			continue
		}

		field := templates.Field{
			Name: utils.Capitalize(parts[0]),
			Type: parts[1],
		}

		// Parse tags
		if len(parts) > 2 {
			tags := strings.Split(parts[2], ",")
			for _, tag := range tags {
				switch tag {
				case "unique":
					field.Unique = true
				case "required":
					field.Required = true
				case "index":
					field.Index = true
				}
			}
		}

		fields = append(fields, field)
	}

	return fields
}
