package molog

import (
	"log"
	"os"
)

var InfoLog = log.New(os.Stdout, "INFO ", log.Flags())
var WarnLog = log.New(os.Stdout, "WARN ", log.Flags())
var StatsLog = log.New(os.Stdout, "STATS ", log.Flags())
var TraceLog = log.New(os.Stdout, "TRACE ", log.Flags())
var ErrorLog = log.New(os.Stderr, "ERROR ", log.Flags())

