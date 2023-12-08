package logger

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

var INFO = "[INFO]"
var ERROR = "[ERROR]"
var WARNING = "[WARNING]"
var MESSAGE = "[MESSAGE]"
var DEBUG = "[DEBUG]"

var timeStampFormat = "2006/01/02 15:04:05"

// Simply reports to the console
func logMsg(prefix, s string) {
	timeStamp := time.Now().Format(timeStampFormat)
	fmt.Printf("%s %s %s\n", timeStamp, prefix, s)
}

// Some simple formatter shortcuts
func Info(s string)    { logMsg(INFO, s) }
func Error(s string)   { logMsg(ERROR, s) }
func Warning(s string) { logMsg(WARNING, s) }
func Message(s string) { logMsg(MESSAGE, s) }
func PrintDivide(debugFlag bool) {
	if debugFlag && viper.GetString("ENVIRONMENT") == "development" {
		fmt.Println("============================================================================")
	} else if !debugFlag {
		fmt.Println("============================================================================")
	}
}

// Enables additional logging in local/impersonation sessions
func Debug(s string) {
	if viper.GetString("ENVIRONMENT") == "development" {
		str := s
		logMsg(DEBUG, str)
	}
}
