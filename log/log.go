package molog

import (
	"log"
	"os"
)

var infoLog = log.New(os.Stdout, "INFO ", log.Flags())
var warnLog = log.New(os.Stdout, "WARN ", log.Flags())
var errorLog = log.New(os.Stderr, "ERROR ", log.Flags())

func Errorf(format string, params ...interface{}) {
	errorLog.Printf(format+"\n", params)
}

func Infof(format string, params ...interface{}) {
	infoLog.Printf(format+"\n", params)
}

func Infoln(format string) {
	infoLog.Println(format)
}

func Warnf(format string, params ...interface{}) {
	warnLog.Printf(format+"\n", params)
}
