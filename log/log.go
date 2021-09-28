package log

import (
	"log"
	"os"
)

var Info = log.New(os.Stdout, "INFO ", log.Flags()|log.Lshortfile)
var Warn = log.New(os.Stdout, "WARN ", log.Flags()|log.Lshortfile)
var Err = log.New(os.Stderr, "ERROR ", log.Flags()|log.Lshortfile)
