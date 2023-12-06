package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jlammilliman/dbManager/pkg/config"
)

func main() {
	conf, err := config.LoadConfig()
	if err != nil {
		logger.Error(fmt.Sprintf("Error loading config: %v", err))
	}
	config.LogConfig(conf)
}
