package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/jinzhu/inflection"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var modelTemplate = `package models

{{if .ModelImports}}
import (
{{range .ModelImports}}
	"{{.}}"
{{end}}
)
{{end}}    


type {{.TableName}} struct {
{{- range .Columns }}
    {{.Name}} {{.Type}} ` + "`gorm:\"column:{{.GormName}}\"`" + `
{{- end }}
}

func ({{.TableName}}) TableName() string {
    return "{{.DBTableName}}"
}
`

type Column struct {
	Name     string
	GormName string
	Type     string
}

type Table struct {
	TableName    string
	DBTableName  string
	Columns      []Column
	ModelImports []string
}

func main() {
	destPath := flag.String("dest", ".", "Destination path for generated models")
	envFile := flag.String("env", "", "Path to .env file")
	dbUser := flag.String("dbuser", "", "Database user")
	dbPassword := flag.String("dbpassword", "", "Database password")
	dbHost := flag.String("dbhost", "", "Database host")
	dbPort := flag.String("dbport", "", "Database port")
	dbName := flag.String("dbname", "", "Database name")
	tables := flag.String("tables", "", "Comma-separated list of tables to generate models for")
	flag.Parse()

	// Load environment variables from .env file if it exists
	if _, err := os.Stat(*envFile); err == nil {
		err := godotenv.Load(*envFile)
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}
	}

	// Override environment variables with command-line arguments if provided
	if *dbUser == "" {
		*dbUser = os.Getenv("DB_USER")
	}
	if *dbPassword == "" {
		*dbPassword = os.Getenv("DB_PASSWORD")
	}
	if *dbHost == "" {
		*dbHost = os.Getenv("DB_HOST")
	}
	if *dbPort == "" {
		*dbPort = os.Getenv("DB_PORT")
	}
	if *dbName == "" {
		*dbName = os.Getenv("DB_NAME")
	}
	if *tables == "" {
		*tables = os.Getenv("TABLES")
	}

	if *dbUser == "" || *dbPassword == "" || *dbName == "" || *tables == "" {
		log.Fatal("Database user, password, name, and tables are required")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", *dbUser, *dbPassword, *dbHost, *dbPort, *dbName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	tableNames := strings.Split(*tables, ",")
	for _, tableName := range tableNames {
		generateModel(db, tableName, *destPath)
	}
}

func generateModel(db *gorm.DB, tableName, destPath string) {
	var columns []Column
	var modelImports []string
	columnTypes, err := db.Migrator().ColumnTypes(tableName)
	if err != nil {
		log.Fatalf("Failed to get columns for table %s: %v", tableName, err)
	}

	for _, columnType := range columnTypes {
		modelColumnType := columnType.DatabaseTypeName()
		// Add special handling for datetime columns
		switch columnType.DatabaseTypeName() {
		case "datetime", "timestamp", "date", "time":
			modelColumnType = "time.Time"
			if !strings.Contains(strings.Join(modelImports, ","), "time") {
				modelImports = append(modelImports, "time")
			}
		case "tinyint", "smallint", "mediumint", "int", "integer", "bigint":
			modelColumnType = "int"
		case "float", "double", "real":
			modelColumnType = "float64"
		case "decimal", "numeric":
			modelColumnType = "string" // or use a custom decimal type
		case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
			modelColumnType = "string"
		case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
			modelColumnType = "[]byte"
		case "bit":
			modelColumnType = "[]uint8"
		case "bool", "boolean":
			modelColumnType = "bool"
		case "json":
			modelColumnType = "json.RawMessage"
			if !strings.Contains(strings.Join(modelImports, ","), "encoding/json") {
				modelImports = append(modelImports, "encoding/json")
			}
		case "enum", "set":
			modelColumnType = "string"
		default:
			modelColumnType = "string" // default to string for any other types
		}

		column := Column{
			Name:     camelCase(columnType.Name()),
			Type:     modelColumnType,
			GormName: columnType.Name(),
			// Add other fields as necessary
		}
		columns = append(columns, column)
	}

	// depluralize table name
	depluraizedTableName := inflection.Singular(tableName)

	table := Table{
		TableName:    camelCase(depluraizedTableName),
		Columns:      columns,
		DBTableName:  tableName,
		ModelImports: modelImports,
	}

	tmpl, err := template.New("model").Parse(modelTemplate)
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	file, err := os.Create(fmt.Sprintf("%s/%s.go", destPath, table.TableName))
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	err = tmpl.Execute(file, table)
	if err != nil {
		log.Fatalf("Failed to execute template: %v", err)
	}
}

func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}
