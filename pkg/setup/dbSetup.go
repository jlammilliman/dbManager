package setup

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

// ! THIS IS INTENDED FOR A LOCAL DEVELOPMENT ENVIRONMENT ONLY.
// Be HYPER CAUTIOUS about allowing the code to tickle prod in such a way

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
		log.Println("Failed to lookup database.")
		log.Fatal(err)
	}

	for results.Next() {
		var dbName string
		err := results.Scan(&dbName)
		if err != nil {
			log.Fatal(err)
		}

		if dbName == database {
			log.Println("Database already exists.")

			if forceRefresh {
				log.Printf("Force refresh is enabled. Dropping the database: %s.\n", database)

				// Disconnect all users from the database
				log.Printf("Clearing connections on %s\n", database)
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
				log.Println("Database dropped.")

				// Reset multi-user connection setting
				setMultiUserQuery := fmt.Sprintf("ALTER DATABASE [%s] SET MULTI_USER;", database)
				_, err = db.Exec(setMultiUserQuery)
				if err != nil {
					// It's okay to ignore this error since we just dropped the database
					log.Println("Ignoring Error: Could not set database to MULTI_USER, which is expected since the database was just dropped.")
				}

			} else {
				// If you don't want to force-refresh, just return
				return nil
			}
		}
	}

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

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config %v", err)
	}
	// Define connection details
	server := config.DB.Host
	port := config.DB.Port
	username := "sa"                    // Default login from SQLserver
	password := "Test@123"              // If you modify this you MUST modify it in the dockerfile
	database := config.DB.Name          // Name of the database it will spin up in the container
	containerName := "roadconductor_db" // Name of the container in Docker

	// Account for the API to access the DB
	userUsername := config.DB.Username
	userPassword := config.DB.Password

	// Driver Logic Below

	// Check for --force-refresh flag
	forceRefresh := false
	for _, arg := range os.Args {
		if arg == "--force-refresh" {
			forceRefresh = true
			break
		}
	}

	// Check to see if we have an active DB Connection w/ given creds
	// IFF we do, then we don't have to run with the docker shenanigans
	isDBConnected := isSQLServerContainerReady(server, port, username, password)
	if !isDBConnected {
		log.Println("No active DB connection found. Checking for docker setup...")
		// Check for docker container
		containerRunning := isContainerRunning(containerName)
		if containerRunning {
			log.Printf("Container '%s' is already running.", containerName)
		} else {
			log.Printf("Container '%s' is not running. Starting it now...", containerName)

			// Run docker compose
			cmd := exec.Command("docker-compose", "-f", "scripts/local-db-setup/docker-compose.yml", "up", "-d", "--force-recreate")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Fatalf("Failed to start the Docker container: %v", err)
			}
			log.Println("Docker container up.")
		}
		log.Println("============================================================")
	}

	// Wait for the SQL Server container to start up
	log.Println("Starting Database setup...")
	for !isSQLServerContainerReady(server, port, username, password) {
		log.Println("Waiting for SQL Server to start...")
		time.Sleep(5 * time.Second)
	}
	log.Println("============================================================")
	log.Println("Connected to SQL Server.")

	// Create the dev database
	log.Printf("Creating %s database...", database)
	err = createDatabase(server, port, username, password, database, forceRefresh)
	if err != nil {
		log.Fatalf("Failed to create %s database: %v", database, err)
	}

	// Execute the init.sql file to populate the database
	log.Println("Running init...")
	err = executeSQLFile(server, port, username, password, database, "./scripts/local-db-setup/init.sql")
	if err != nil {
		log.Fatalf("Failed to execute init.sql: %v", err)
	}

	log.Println("Running setup...")
	err = executeSQLFile(server, port, username, password, database, "./scripts/local-db-setup/setup.sql")
	if err != nil {
		log.Fatalf("Failed to execute setup.sql: %v", err)
	}

	log.Printf("Creating programmatic access read/write user on %s...", database)
	err = createReadWriteUser(server, port, username, password, database, userUsername, userPassword)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	log.Println("============================================================")
	log.Println("Database setup completed.")
}
