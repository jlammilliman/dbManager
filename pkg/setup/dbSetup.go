package setup

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/jlammilliman/dbManager/pkg/config"
	"github.com/jlammilliman/dbManager/pkg/logger"
	_ "github.com/denisenkom/go-mssqldb"
)

// ! THIS IS INTENDED FOR A LOCAL DEVELOPMENT ENVIRONMENT ONLY.
// Be HYPER CAUTIOUS about allowing the code to tickle prod in such a way

func Exec(config *config.Config, forceRefresh bool) {

	// Define connection details
	server := config.TargetDB.Host
	port := config.TargetDB.Port
	username := "sa"                	// Default login from SQLserver
	password := "Test@123"          	// If you modify this you MUST modify it in the dockerfile
	database := config.TargetDB.Name	// Name of the database it will spin up in the container
	containerName := config.DockerContainer 

	// User Account for DB access to TargetDB.Name
	userUsername := config.TargetDB.Username
	userPassword := config.TargetDB.Password

	// Check to see if we have an active DB Connection w/ given creds
	// IFF we do, then we don't have to run with the docker shenanigans
	isDBConnected := isSQLServerContainerReady(server, port, username, password)
	if !isDBConnected {
		logger.Warning("No active DB connection found. Checking for docker setup...")
		// Check for docker container
		containerRunning := isContainerRunning(containerName)
		if containerRunning {
			logger.Info(fmt.Sprintf("Container '%s' is already running.", containerName))
		} else {
			logger.Debug(fmt.Sprintf("Container '%s' is not running. Starting it now...", containerName))

			// Run docker compose
			cmd := exec.Command("docker-compose", "-f", "scripts/local-db-setup/docker-compose.yml", "up", "-d", "--force-recreate")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				logger.Error(fmt.Sprintf("Failed to start the Docker container: %v", err))
			}
			logger.Info("Docker container up.\n")
		}
	}

	// Wait for the SQL Server container to start up
	logger.Debug("Starting Database setup...")
	for !isSQLServerContainerReady(server, port, username, password) {
		logger.Debug("Waiting for SQL Server to start...")
		time.Sleep(5 * time.Second)
	}
	logger.Info("Connected to SQL Server.")

	// Create the dev database
	logger.Debug(fmt.Sprintf("Creating %s database...", database))
	err := createDatabase(server, port, username, password, database, forceRefresh)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create %s database: %v", database, err))
		return
	}

	// Lookup the database schema to setup the local database. If there is none, we die here
    err = checkDatabaseDirs(database)
    if err != nil {
        logger.Error(fmt.Sprintf("Error verifying local generation schema. Error: %v",err))
        logger.Message("If Schema does not exist, try running with '--generate' or '-g' to create from a source database.")
		return
    } else {
        logger.Debug("Local generation schema exists.")
    }

	/*
		A proper local Database copy setup is as follows:
			- All Tables exist locally
			- All Functions exist locally
			- All Views exist locally
			- All Procedures exist locally
			- A user account exists with read/write access to the database

		Where this gets challenging is creating a propogation strategy. To do this, we assume that the generation
		process creates an in order schema for us to hydrate the database with. 
	*/

	baseDir := fmt.Sprintf("/databases/%s", database)

	// TABLES
	logger.Debug("Creating tables...")
	tableFiles, err := listFiles(fmt.Sprintf("%s/tables", baseDir))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to load table schema: %v", err))
		return
	}

	err = executeSQLFiles(server, port, username, password, database, tableFiles)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to execute init.sql: %v", err))
		return
	}

	// VIEWS
	logger.Debug("Creating Views...")
	viewFiles, err := listFiles(fmt.Sprintf("%s/views", baseDir))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to load view schema: %v", err))
		return
	}

	err = executeSQLFiles(server, port, username, password, database, viewFiles)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to execute init.sql: %v", err))
		return
	}

	// FUNCTIONS
	logger.Debug("Creating Functions...")
	functionFiles, err := listFiles(fmt.Sprintf("%s/functions", baseDir))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to load function schema: %v", err))
		return
	}

	err = executeSQLFiles(server, port, username, password, database, functionFiles)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to execute init.sql: %v", err))
		return
	}

	// PROCEDURES
	logger.Debug("Creating Procedures...")
	procedureFiles, err := listFiles(fmt.Sprintf("%s/procedures", baseDir))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to load procedure schema: %v", err))
		return
	}

	err = executeSQLFiles(server, port, username, password, database, procedureFiles)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to execute init.sql: %v", err))
		return
	}

	// CREATE USER FOR DATABASE
	logger.Debug(fmt.Sprintf("Creating read/write user on %s...", database))
	err = createReadWriteUser(server, port, username, password, database, userUsername, userPassword)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create user: %v", err))
		return
	}

	logger.Info("Database setup completed.")
}
