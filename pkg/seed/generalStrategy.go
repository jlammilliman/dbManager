package seed

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/jlammilliman/dbManager/pkg/logger"
)

/*
	If we do not have a specified stratefy for seeding a table, it will default to use this strategy.

	We will handle column types, and insert statement building in here to seed as many times as we are called.
	When called, specify a tableDetails object, and this script will handle the rest.
*/

type ColumnDetails struct {
	Name             string
	Type             string
	IsPrimaryKey     bool
	ReferencedTable  string
	ReferencedColumn string
	ColumnSize       int
	ColumnDefault    string // This helps us grab identities, or default values so we can ignore columns or override a Pkey insert
}

type TableDetails struct {
	TableName string
	Columns   []ColumnDetails
	NumSeeds  int
}

func CallGeneralStrategy(db *sql.DB, tableDetails TableDetails) error {
	gofakeit.Seed(0) // Initialize gofakeit

	// Not the most efficient way, but this is a local database seed script sooooo.....
	for i := 0; i < tableDetails.NumSeeds; i++ {
		columnNames := []string{}
		valueHolders := []string{}
		var values []interface{}

		paramCounter := 1

		for _, col := range tableDetails.Columns {
			// Exclude primary keys. Include primary keys that are foreign keys, include primary keys without an identity
			if (col.ReferencedTable == "" || col.ReferencedColumn == "" || col.ColumnDefault != "") && col.IsPrimaryKey {
				continue
			} else {
				// If we escaped the above continue, we have a primary key without an identity
				// Meaning we need to propogate the primary key...
				logger.Debug(fmt.Sprintf("Primary Key '%s', Type: '%s', does not have an identity.", col.Name, col.Type))
				if col.Type == "int" {
					// Fetch the maximum value of the primary key from the database and increment it
					var maxID int
					maxQuery := fmt.Sprintf("SELECT MAX(%s) FROM %s", col.Name, tableDetails.TableName)
					err := db.QueryRow(maxQuery).Scan(&maxID)
					if err != nil && err != sql.ErrNoRows {
						return err
					}
					values = append(values, maxID+1)
					columnNames = append(columnNames, col.Name)
					valueHolders = append(valueHolders, fmt.Sprintf("@p%d", paramCounter))
					paramCounter++
					logger.Debug(fmt.Sprintf("Found next int value: %d", maxID+1))
					continue
				}
				// If for some reason the primary key is not an int ID, we'll bottom out to find a good fake data value
			}
			columnNames = append(columnNames, col.Name)

			// If we are a foreign key, grab a suitable value
			if col.ReferencedTable != "" {
				// Fetch a random foreign key from the referenced table

				// We can assign this to default user, since the seed script should ONLY be used in local dev....
				if col.Name == "createdBy" || col.Name == "updatedBy" {
					values = append(values, 1) // Seeder should only ever be used locally, 1 is (usually) default admin account
				} else {
					query := fmt.Sprintf("SELECT TOP 1 %s FROM %s ORDER BY NEWID()", col.ReferencedColumn, col.ReferencedTable)
					row := db.QueryRow(query)
					var fkValue int
					if err := row.Scan(&fkValue); err != nil {
						return err
					}
					values = append(values, fkValue)
				}
			} else {
				// Use type-based logic to generate some garbage
				logger.Debug(fmt.Sprintf("Matching Column: '%s', Type: '%s'", col.Name, col.Type))
				switch strings.ToLower(col.Type) {
				case "bigint", "int", "smallint", "tinyint":
					values = append(values, gofakeit.Number(0, 10000))

				case "bit":
					values = append(values, gofakeit.Bool())

				case "decimal", "numeric", "money", "smallmoney":
					values = append(values, gofakeit.Float32Range(0, 10000))

				case "float":
					values = append(values, gofakeit.Float64())

				case "real":
					values = append(values, gofakeit.Float32())

				case "date":
					values = append(values, gofakeit.Date())

				case "datetime", "datetime2", "smalldatetime":
					values = append(values, gofakeit.Date())

				case "datetimeoffset":
					values = append(values, gofakeit.Date().Format(time.RFC3339))

				case "time":
					values = append(values, gofakeit.Date().Format("15:04:05"))

				case "char", "varchar", "text":
					values = append(values, gofakeit.Sentence(5))

				case "nchar", "nvarchar", "ntext":
					values = append(values, gofakeit.Sentence(5))

				case "binary", "varbinary":
					values = append(values, gofakeit.City()) // Just dump something in there

				case "image":
					values = append(values, gofakeit.ImageURL(100, 100))

				case "cursor":
					// Cursors are not typically used in data seeding
					continue

				case "hierarchyid":
					values = append(values, fmt.Sprintf("/%d/", gofakeit.Number(1, 100)))

				case "sql_variant":
					values = append(values, gofakeit.Word())

				case "table":
					// This is almost never used, skipping
					continue

				case "timestamp":
					values = append(values, gofakeit.Date())

				case "uniqueidentifier":
					values = append(values, gofakeit.UUID())

				case "xml":
					values = append(values, fmt.Sprintf("<root><value>%s</value></root>", gofakeit.Word()))

				case "json":
					values = append(values, fmt.Sprintf("{\"key\": \"%s\"}", gofakeit.Word()))

				case "geometry", "geography":
					// Generate a random point for geometry/geography types
					values = append(values, fmt.Sprintf("POINT(%f %f)", gofakeit.Longitude(), gofakeit.Latitude()))

				// Specialized String Types
				case "sysname":
					values = append(values, gofakeit.Username())

				default:
					logger.Error(fmt.Sprintf("UNHANDLED TYPE: Column: '%s', Type: '%s'", col.Name, strings.ToLower(col.Type)))
					values = append(values, gofakeit.Word()) // Default case
				}

			}
			valueHolders = append(valueHolders, fmt.Sprintf("@p%d", paramCounter))
			paramCounter++
		}

		// Handle the case where we filtered out all columns (SomEhOw)
		if len(values) == 0 {
			logger.Info(fmt.Sprintf("SKIPPED SEEDING ON '%s'. No Values were generated!\n", tableDetails.TableName))
		} else {
			query := fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s)",
				tableDetails.TableName,
				strings.Join(columnNames, ", "),
				strings.Join(valueHolders, ", "),
			)

			logger.Debug(fmt.Sprintf("Generated Query for '%s':\n  [QUERY]: %s", tableDetails.TableName, query))
			logger.PrintDivide(true)
			logger.Debug(fmt.Sprintf(" [VALUES]: %v", values))
			logger.PrintDivide(true)

			_, err := db.Exec(query, values...)
			if err != nil {
				return err
			}
		}
	}
	logger.Info(fmt.Sprintf("SEEDED: '%s' %d times.", tableDetails.TableName, tableDetails.NumSeeds))

	return nil
}
