package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type DB struct {
	Host     string
	Port     string
	Name     string
	Username string
	Password string
}

type Config struct {
	Environment string

	SourceDB DB
	TargetDB DB

	HasSource bool
	HasTarget bool
}

func init() {}

func initConfig() error {
	viper.AutomaticEnv()
	viper.SetConfigFile(".env")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading .env file, %v", err)
	}
	return nil
}

func loadConfig() (*Config, error) {
	initConfig()

	sourceDB := &DB{
		Host:     viper.GetString("DB_SOURCE_HOST"),
		Port:     viper.GetString("DB_SOURCE_PORT"),
		Name:     viper.GetString("DB_SOURCE_NAME"),
		Username: viper.GetString("DB_SOURCE_USERNAME"),
		Password: viper.GetString("DB_SOURCE_PASSWORD"),
	}

	targetDB := &DB{
		Host:     viper.GetString("DB_TARGET_HOST"),
		Port:     viper.GetString("DB_TARGET_PORT"),
		Name:     viper.GetString("DB_TARGET_NAME"),
		Username: viper.GetString("DB_TARGET_USERNAME"),
		Password: viper.GetString("DB_TARGET_PASSWORD"),
	}

	// Sanity check the env: Base DB values
	hasSourceDB := sourceDB.Host != "" && sourceDB.Port != "" && sourceDB.Name != ""
	hasTargetDB := targetDB.Host != "" && targetDB.Port != "" && targetDB.Name != ""

	// If target or source provided, check that username and password is provided
	if hasSourceDB && (sourceDB.Username == "" || sourceDB.Password == "") {
		return nil, fmt.Errorf("no DB credentials supplied for SOURCE")
	}

	if hasTargetDB && (targetDB.Username == "" || targetDB.Password == "") {
		return nil, fmt.Errorf("no DB credentials supplied for TARGET")
	}


	config := &Config{
		Environment: viper.GetString("ENVIRONMENT"),
		SourceDB: *sourceDB,
		TargetDB: *targetDB,
		HasSource: hasSourceDB,
		HasTarget: hasTargetDB,
	}

	return config, nil
}

func LogConfig(config *Config) {
	fmt.Println("================================================")
	fmt.Printf(" ENVIRONMENT    	: %s\n", config.Environment)
	if config.HasSource {
		fmt.Printf(" SOURCE HOST        : %s:%s\n", config.TargetDB.Host, config.TargetDB.Port)
		fmt.Printf(" SOURCE Name        : %s\n", config.TargetDB.Name)
		fmt.Printf(" SOURCE User        : %s\n", config.TargetDB.Username)
	}
	if config.HasTarget {
		fmt.Printf(" TARGET HOST        : %s:%s\n", config.TargetDB.Host, config.TargetDB.Port)
		fmt.Printf(" TARGET Name        : %s\n", config.TargetDB.Name)
		fmt.Printf(" TARGET User        : %s\n", config.TargetDB.Username)
	}
	if !config.HasSource && !config.HasTarget {
		fmt.Println(" No TARGET or SOURCE DB Provided!")
	}
	fmt.Println("================================================")
}

// Export config struct
func LoadConfig() (*Config, error) {
	return loadConfig()
}
