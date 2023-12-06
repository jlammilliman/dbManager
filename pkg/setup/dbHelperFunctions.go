package setup

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"os"
	"os/exec"
	"io/ioutil"
    "path/filepath"

	"github.com/jlammilliman/dbManager/pkg/logger"
	_ "github.com/denisenkom/go-mssqldb"
)

// ==========================
// DOCKER Helpers
// ==========================

// Checks to see if the local docker container is running on the host machine
func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", "/"+containerName)
	output, err := cmd.Output()
	log.Println(output)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// If the SQLServer container has just started, it usually takes a minute or so to spin up depending on local hardware,
// So we wait for it
func isSQLServerContainerReady(server, port, username, password string) bool {
	connString := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=master", server, port, username, password)
	db, err := sql.Open("sqlserver", connString)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer db.Close()

	err = db.Ping()
	return err == nil
}

// ==========================
// File Management Helpers
// ==========================

func checkDirExists(path string) bool {
    info, err := os.Stat(path)
    if os.IsNotExist(err) {
        return false
    }
    return info.IsDir()
}

// Sanity check for the required directories to exist in a local generation
func checkDatabaseDirs(dbName string) error {
    baseDir := filepath.Join("databases", dbName)
    subDirs := []string{"tables", "functions", "procedures", "views", "seedStrategies"}

    for _, dir := range subDirs {
        fullPath := filepath.Join(baseDir, dir)
        if !checkDirExists(fullPath) {
            return fmt.Errorf("directory not found: %s", fullPath)
        }
    }
    return nil
}

// ListFiles lists all files in a given directory.
func listFiles(directory string) ([]string, error) {
    var files []string
    items, err := ioutil.ReadDir(directory)
    if err != nil {
        return nil, err
    }

    for _, item := range items {
        if !item.IsDir() {
            files = append(files, filepath.Join(directory, item.Name()))
        }
    }

    return files, nil
}

// ==========================
// MS SQL SERVER Helpers
// ==========================

func executeSQLFiles(server, port, username, password, database string, files []string) error {
    for _, file := range files {
        err := executeSQLFile(server, port, username, password, database, file)
        if err != nil {
            return err
        }
		logger.Debug(fmt.Sprintf("Executed '%v'", file))
    }
	return nil
}

// Spins up a database, optionally dropping it if it already exists
func createDatabase(server, port, username, password, database string, forceRefresh bool) error {
	connString := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=master", server, port, username, password)
	db, err := sql.Open("sqlserver", connString)
	if err != nil {
		return err
	}
	defer db.Close()

	checkDBQuery := fmt.Sprintf("SELECT name FROM sys.databases WHERE name = '%s'", database)
	results, err := db.Query(checkDBQuery)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to lookup database. Error: %v", err))
	}

	for results.Next() {
		var dbName string
		err := results.Scan(&dbName)
		if err != nil {
			logger.Error(fmt.Sprintf("Error in Scan: %v", err))
		}

		if dbName == database {
			logger.Debug("Database already exists.")

			if forceRefresh {
				logger.Warning(fmt.Sprintf("Force refresh is enabled. Dropping the database: %s.\n", database))

				// Disconnect all users from the database
				logger.Debug(fmt.Sprintf("Clearing connections on %s", database))
				disconnectUsersQuery := fmt.Sprintf(`
					USE master;
					ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;`, database)
				_, err := db.Exec(disconnectUsersQuery)
				if err != nil {
					return err
				}

				// Drop
				dropDBQuery := fmt.Sprintf("DROP DATABASE %s;", database)
				_, err = db.Exec(dropDBQuery)
				if err != nil {
					return err
				}
				logger.Info("Database dropped.")

				// Reset multi-user connection setting
				setMultiUserQuery := fmt.Sprintf("ALTER DATABASE [%s] SET MULTI_USER;", database)
				_, err = db.Exec(setMultiUserQuery)
				if err != nil {
					// It's okay to ignore this error since we just dropped the database
					logger.Info("Ignoring Error: Could not set database to MULTI_USER, which is expected since the database was just dropped.")
				}

			} else {
				// If you don't want to force-refresh, just return
				logger.Debug(fmt.Sprintf("Database '%s' already exists. If you'd like to force drop and repropogate, run with '--force' or '-f'", dbName))
				return nil
			}
		}
	}

	logger.Debug(fmt.Sprintf("Creating Database '%s'.", database))
	createDBQuery := fmt.Sprintf("CREATE DATABASE %s;", database)
	_, err = db.Exec(createDBQuery)
	return err
}

// Helper function to handle running .sql files
func executeSQLFile(server, port, username, password, database, filePath string) error {
	// Read the SQL file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read SQL file: %v", err)
	}

	// Convert the content to string
	sqlScript := string(content)

	// Split the script into separate SQL statements
	sqlStatements := strings.Split(sqlScript, ";")

	// Establish a database connection
	connString := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=%s", server, port, username, password, database)
	db, err := sql.Open("mssql", connString)
	if err != nil {
		return fmt.Errorf("failed to connect to the database: %v", err)
	}
	defer db.Close()

	// Execute each SQL statement
	for _, statement := range sqlStatements {
		statement = strings.TrimSpace(statement)
		if statement != "" {
			_, err = db.Exec(statement)
			if err != nil {
				return fmt.Errorf("failed to execute SQL statement: %v", err)
			}
		}
	}

	return nil
}

func createReadWriteUser(server, port, username, password, database, userUsername, userPassword string) error {
	// CREATE LOGIN statement
	createLoginCmd := exec.Command("sqlcmd", "-S", fmt.Sprintf("%s,%s", server, port),
		"-U", username, "-P", password, "-d", database,
		"-Q", fmt.Sprintf("CREATE LOGIN %s WITH PASSWORD='%s';", userUsername, userPassword))
	err := createLoginCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create login: %v", err)
	}

	// CREATE USER statement
	createUserCmd := exec.Command("sqlcmd", "-S", fmt.Sprintf("%s,%s", server, port),
		"-U", username, "-P", password, "-d", database,
		"-Q", fmt.Sprintf("USE %s; CREATE USER %s FOR LOGIN %s;", database, userUsername, userUsername))
	err = createUserCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	// ADD MEMBER to db_datareader role
	addReaderRoleCmd := exec.Command("sqlcmd", "-S", fmt.Sprintf("%s,%s", server, port),
		"-U", username, "-P", password, "-d", database,
		"-Q", fmt.Sprintf("USE %s; ALTER ROLE db_datareader ADD MEMBER %s;", database, userUsername))
	err = addReaderRoleCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add user to db_datareader role: %v", err)
	}

	// ADD MEMBER to db_datawriter role
	addWriterRoleCmd := exec.Command("sqlcmd", "-S", fmt.Sprintf("%s,%s", server, port),
		"-U", username, "-P", password, "-d", database,
		"-Q", fmt.Sprintf("USE %s; ALTER ROLE db_datawriter ADD MEMBER %s;", database, userUsername))
	err = addWriterRoleCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add user to db_datawriter role: %v", err)
	}

	return nil
}