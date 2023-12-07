package seed

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

/**
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
}

type TableDetails struct {
	TableName string
	Columns   []ColumnDetails
	NumSeeds  int
}

func CallGeneralStrategy(db *sql.DB, tableDetails TableDetails) error {
	rand.Seed(time.Now().UnixNano()) // Seed the random number generator (Ignoring deprecation warning because it literally doesn't matter, this is dummy data)

	// Not the most efficient way, but this is a local database seed script sooooo.....
	for i := 0; i < tableDetails.NumSeeds; i++ {
		columnNames := []string{}
		valueHolders := []string{}
		var values []interface{}

		paramCounter := 1

		for _, col := range tableDetails.Columns {
			// Exclude primary keys. Include primary keys that are foreign keys
			if ((col.ReferencedTable == "" || col.ReferencedColumn == "") && col.IsPrimaryKey) || col.Name == "createdAt" || col.Name == "updatedAt" {
				continue
			}

			columnNames = append(columnNames, col.Name)

			// If we are a foreign key, grab a suitable value
			if col.ReferencedTable != "" {
				// Fetch a random foreign key from the referenced table

				// We can assign this to default user, since the seed script should ONLY be used in local dev....
				if col.Name == "createdBy" || col.Name == "updatedBy" {
					values = append(values, 1) // Seeder should only ever be used locally, 1 is default admin account
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
				switch strings.ToLower(col.Type) {
				case "varchar", "nvarchar", "text": // AKA Strings

					// We defined a bunch of safe for work strings to use in 'mockData.go'
					if col.Name == "firstname" {
						values = append(values, FirstNames[rand.Intn(len(FirstNames))])

					} else if col.Name == "lastname" {
						values = append(values, LastNames[rand.Intn(len(LastNames))])

					} else if col.Name == "email" {
						values = append(values, Emails[rand.Intn(len(Emails))])

					} else if col.Name == "phoneNumber" {
						values = append(values, PhoneNumbers[rand.Intn(len(PhoneNumbers))])

					} else {
						values = append(values, "Default string. We can do better")
					}

				case "int":
					if col.Name == "createdBy" || col.Name == "updatedBy" {
						values = append(values, 1) // Seeder should only ever be used locally, 1 is default admin account
					} else {
						values = append(values, rand.Int())
					}

				case "bit":
					values = append(values, rand.Intn(2) == 1) // Gets a random bit

				default:
					values = append(values, rand.Int())
					fmt.Println("UNHANDLED TYPE:", strings.ToLower(col.Type))
				}
			}
			valueHolders = append(valueHolders, fmt.Sprintf("@p%d", paramCounter))
			paramCounter++
		}

		// Handle the case where we filtered out all columns (SomEhOw)
		if len(values) == 0 {
			fmt.Printf("SKIPPED SEEDING ON '%s'. No Values were generated!\n", tableDetails.TableName)
		} else {
			query := fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s)",
				tableDetails.TableName,
				strings.Join(columnNames, ", "),
				strings.Join(valueHolders, ", "),
			)

			_, err := db.Exec(query, values...)
			if err != nil {
				return err
			}
		}
	}
	fmt.Printf("SEEDED: '%s'\n", tableDetails.TableName)

	return nil
}
