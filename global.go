package main

import (
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

const (
	AuthToken = "X-Auth-Token"
)

type Args struct {
	Mode        string `arg:"required"`
	Secret      string
	ClientCount int
	ServerUrl   string
	ListenPort  int
	Verbose     bool
}

var args = &Args{}
