package setup

import (
	"database/sql"
	"fmt"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jlammilliman/dbManager/pkg/config"
	"github.com/jlammilliman/dbManager/pkg/logger"
)

// This handles the generation of a local sourceDatabase schema given a source sourceDatabase

// This list will be used to filter out any tables that we absolutely do not want to seed
var BlockedFromSeeding []string = []string{
	"Roles",
	"Users",
}

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

func Generate(config *config.Config, forceRefresh bool) {

	server := config.SourceDB.Host
	port := config.SourceDB.Port
	sourceDatabase := config.SourceDB.Name
	targetDatabase := config.SourceDB.Name
	username := config.SourceDB.Username
	password := config.SourceDB.Password

	// If we have a target database specified, name the directory after the target db name
	if config.HasTarget {
		targetDatabase = config.TargetDB.Name
	}

	isDBConnected := isSQLServerContainerReady(server, port, username, password)
	if !isDBConnected {
		if config.UsingDocker {
			logger.Warning("No active DB connection found. Checking for docker setup...")
		}
		logger.Error("No active DB connection found.")
		return
	}

	logger.Debug(fmt.Sprintf("Opening connection to '%s' at '%s:%s'...", sourceDatabase, server, port))
	connString := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=%s", server, port, username, password, sourceDatabase)
	db, err := sql.Open("sqlserver", connString)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to open sourceDatabase '%s': %v\n", sourceDatabase, err))
	}
	defer db.Close()

	// Fetch tables from DB
	tables, err := getTables(db, sourceDatabase)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to get tables: %v", err))
	}

	// Call topography (returns a sorted priority seed list)
	sortedTables, err := sortTables(tables)
	if err != nil {
		logger.Error(fmt.Sprintf("Error Sorting: %v\n", err))
		return
	}

	// Create the .sql files for each table, view, procedure,
	logger.Debug(fmt.Sprintf("Finished sorting tables. Generating schema at 'databases/%s'...", targetDatabase))
	for _, table := range sortedTables {
		logger.Debug(fmt.Sprintf("Table: %s", table.TableName))
	}
}

func getSchema(db *sql.DB, sourceDatabase string) ([]TableDetails, error) {

	query := `
		SELECT 
			CASE 
				WHEN o.type = 'P' THEN 'Procedure'
				WHEN o.type = 'FN' THEN 'Function'
				WHEN o.type = 'V' THEN 'View'
			END AS ObjectType,
			s.name AS SchemaName,
			o.name AS ObjectName,
			re.referenced_entity_name,
			re.referenced_minor_name,
			CASE 
				WHEN re.referenced_minor_id = 0 THEN 'Object'
				ELSE 'Column'
			END AS ReferencedType
		FROM 
			sys.objects o
			INNER JOIN sys.schemas s ON o.schema_id = s.schema_id
			INNER JOIN sys.sql_modules m ON o.object_id = m.object_id
			CROSS APPLY sys.dm_sql_referenced_entities(s.name + '.' + o.name, 'OBJECT') re
		WHERE 
			o.type IN ('P', 'FN', 'V')
	;`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return nil, nil
}

// Query to get all tables and their columns, contraints
func getTables(db *sql.DB, sourceDatabase string) ([]TableDetails, error) {

	// Try not to touch too much. This bad boi is a monolithic query
	query := `
	-- Generate temp tables to assist with the SQL side topographical sorting
	WITH PrimaryKeys AS (
		SELECT 
			tc.TABLE_NAME,
			kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu 
			ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME 
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
	),
	ForeignKeys AS (
		SELECT 
			kcu.TABLE_NAME, 
			kcu.COLUMN_NAME, 
			rc1.UNIQUE_CONSTRAINT_NAME AS REFERENCED_CONSTRAINT_NAME,
			kcu2.TABLE_NAME AS REFERENCED_TABLE_NAME,
			kcu2.COLUMN_NAME AS REFERENCED_COLUMN_NAME
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc1 
			ON kcu.CONSTRAINT_NAME = rc1.CONSTRAINT_NAME
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu2 
			ON rc1.UNIQUE_CONSTRAINT_NAME = kcu2.CONSTRAINT_NAME
	)
	
	SELECT 
		c.TABLE_NAME,
		c.COLUMN_NAME,
		c.DATA_TYPE,
		CASE 
			WHEN pk.COLUMN_NAME IS NOT NULL THEN 'YES'
			ELSE 'NO'
		END AS IS_PRIMARY_KEY,
		fk.REFERENCED_TABLE_NAME,
		fk.REFERENCED_COLUMN_NAME
	FROM INFORMATION_SCHEMA.COLUMNS c
		LEFT JOIN PrimaryKeys pk ON c.TABLE_NAME = pk.TABLE_NAME AND c.COLUMN_NAME = pk.COLUMN_NAME
		LEFT JOIN ForeignKeys fk ON c.TABLE_NAME = fk.TABLE_NAME AND c.COLUMN_NAME = fk.COLUMN_NAME
	WHERE c.TABLE_CATALOG = '` + sourceDatabase + `'
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Pull results into something we can actually use
	tablesMap := make(map[string]*TableDetails)
	for rows.Next() {
		var (
			tableName        string
			columnName       string
			dataType         string
			isPrimaryKeyStr  string
			referencedTable  sql.NullString
			referencedColumn sql.NullString
		)
		err := rows.Scan(&tableName, &columnName, &dataType, &isPrimaryKeyStr, &referencedTable, &referencedColumn)
		if err != nil {
			return nil, err
		}

		if _, exists := tablesMap[tableName]; !exists {
			tablesMap[tableName] = &TableDetails{TableName: tableName}
		}

		column := ColumnDetails{
			Name:             columnName,
			Type:             dataType,
			IsPrimaryKey:     isPrimaryKeyStr == "YES",
			ReferencedTable:  referencedTable.String,
			ReferencedColumn: referencedColumn.String,
		}

		tablesMap[tableName].Columns = append(tablesMap[tableName].Columns, column)
	}

	// Recast to new object to avoid any overlapping/dead data. --> Old array gets garbage collected
	var tables []TableDetails
	for _, table := range tablesMap {
		tables = append(tables, *table)
	}

	logger.Debug(fmt.Sprintf("Successfully retrieved %d tables from '%s'.", len(tables), sourceDatabase))

	return tables, nil
}

/*
	BEGIN sorting. It is more cost efficient to sort in golang than within SQL server
	We use a topological sort to ensure the following:
		1) Tables that have no foreign key constraints are built first
		2) Tables that reference 1 or more tables via foreign key
		   constraint are built after the tables they reference
		3) Tables we specified to be removed from seeding, get removed from list
*/

func topologicalSort(graph map[string][]string) ([]string, error) {
	var order []string
	visited := make(map[string]bool)
	tempStack := make(map[string]bool)

	// Nested node logic
	var visit func(string) error
	visit = func(node string) error {
		if tempStack[node] {
			return fmt.Errorf("cyclic dependency detected. This is indicative of bad DB design")
		}
		if !visited[node] {
			tempStack[node] = true
			for _, dep := range graph[node] {
				if err := visit(dep); err != nil {
					return err
				}
			}
			visited[node] = true
			tempStack[node] = false
			order = append(order, node)
		}
		return nil
	}

	for table := range graph {
		if !visited[table] {
			if err := visit(table); err != nil {
				return nil, err
			}
		}
	}

	return order, nil
}

// Checks for existence of a string in a given array
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// Creates a graph of table dependencies, and returns an ordered list in which it is safe to seed tables
func sortTables(tables []TableDetails) ([]TableDetails, error) {
	graph := make(map[string][]string)

	for _, table := range tables {
		for _, col := range table.Columns {
			if col.ReferencedTable != "" {
				graph[table.TableName] = append(graph[table.TableName], col.ReferencedTable)
			}
		}
	}

	order, err := topologicalSort(graph)
	if err != nil {
		return nil, err
	}

	// Create sorted tableDetails array
	var sortedTables []TableDetails
	for _, tableName := range order {
		for _, table := range tables {
			if table.TableName == tableName {
				isBlocked := stringInSlice(table.TableName, BlockedFromSeeding)
				if !isBlocked {
					table.NumSeeds = 3 // Default number of values to seed for the table
					sortedTables = append(sortedTables, table)
				}
				break
			}
		}
	}

	return sortedTables, nil
}
