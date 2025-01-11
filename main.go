package main

import (
	"github.com/alexflint/go-arg"
	log "github.com/sirupsen/logrus"
)

func main() {
	arg.MustParse(args)

	if args.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	switch args.Mode {
	case "server":
		s := NewServer(args.ServerUrl)
		s.Serve()

	case "client":
		p := NewClientProxy(args.ListenPort, args.ServerUrl, true)
		p.Serve()
	}
}
