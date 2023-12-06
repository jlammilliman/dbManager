package main

import (
	"fmt"
	"os"

	"github.com/jlammilliman/dbManager/pkg/setup"
	"github.com/jlammilliman/dbManager/pkg/config"
	"github.com/jlammilliman/dbManager/pkg/logger"
)

func main() {
	conf, err := config.LoadConfig()
	if err != nil {
		logger.Error(fmt.Sprintf("Error loading config: %v", err))
	}
	config.LogConfig(conf)

	// Configuration options
	force := false			// -- force 	| -f 	--> Warnings default terminate. This enables a pass-through
	runSetup := false		// -- setup 	| -up	--> Tells us to generate a DB from the schema provided onto target
	runGeneration := false 	// -- generate 	| -g	--> Tells us to generate a schema from source DB
	runVerification := false// -- verify	| -v	--> Checks local database schema against source DB
	runSeed := false		// -- seed		| -s	--> Tells us to generate seed data 


	// Check for --force flag
	for _, arg := range os.Args {

		// Force flag enables pass-through on anything that is a warning + break. Displaying the warning, and continuing
		if arg == "--force" || arg == "-f" {
			force = true
			logger.Warning("Running with Force. This disables failsafe checks, use with caution!")
		}
		if arg == "--generate" || arg == "-g" {
			runGeneration = true
			logger.Message("Requested 'Generation'.")
		}
		if arg == "--setup" || arg == "-up" {
			runSetup = true
			logger.Message("Requested 'Setup'.")
		}
		if arg == "--verify" || arg == "-v" {
			runVerification = true
			logger.Message("Requested 'Verification'.")
		}
		if arg == "--seed" || arg == "-s" {
			runSeed = true
			logger.Message("Requested 'Seeding'.")
		}
	}


	if runGeneration {
		if !conf.HasSource {
			logger.Error("Cannot run generation without a provided source DB!")
			logger.Info("Skipping generation process.")
		} else {
			logger.Info(fmt.Sprintf("Generating clone of '%s'...", conf.SourceDB.Name))
			setup.Generate(conf, force)
			logger.Info("Done.")
		}
	}

	// Always run setup, verification, and then seeding in that order
	if !conf.HasTarget {
		logger.Error("Must provide a target database!")
	} else {

		if runSetup {
			logger.Info(fmt.Sprintf("Running Setup for '%s'", conf.TargetDB.Name))
			setup.Exec(conf, force)
		}
		
		if runVerification {
			logger.Info(fmt.Sprintf("Running Verification for '%s'", conf.TargetDB.Name))
			logger.Info("Not Implemented.")
		}
		
		if runSeed {
			logger.Info(fmt.Sprintf("Running Seed for '%s'", conf.TargetDB.Name))
			logger.Info("Not Implemented.")
		}
	}
}
