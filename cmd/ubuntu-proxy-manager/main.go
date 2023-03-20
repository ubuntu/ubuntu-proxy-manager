// Package main is the entry point for the application.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/app"
)

type cmd interface {
	Wait() error
	Quit()
}

func main() {
	c, err := app.New()
	if err != nil {
		log.Errorf("Failed to create app: %v", err)
		os.Exit(1)
	}

	os.Exit(run(c))
}

func run(c cmd) int {
	defer installSignalHandler(c)()

	log.SetFormatter(&log.TextFormatter{
		DisableLevelTruncation: true,
		DisableTimestamp:       true,
	})

	printedUsage, err := parseFlags()
	if printedUsage {
		if err != nil {
			return 2
		}
		return 0
	}

	if err := c.Wait(); err != nil {
		log.Error(err)
		return 1
	}

	return 0
}

func installSignalHandler(a cmd) func() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			switch v, ok := <-c; v {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM:
				a.Quit()
				return
			default:
				// channel was closed: we exited
				if !ok {
					return
				}
			}
		}
	}()

	return func() {
		signal.Stop(c)
		close(c)
		wg.Wait()
	}
}

func parseFlags() (printedUsage bool, err error) {
	var debug, version, help bool

	fSet := flag.NewFlagSet("ubuntu-proxy-manager", flag.ContinueOnError)

	fSet.BoolVar(&debug, "debug", false, "")
	fSet.BoolVar(&debug, "d", false, "")
	fSet.BoolVar(&version, "version", false, "")
	fSet.BoolVar(&version, "v", false, "")
	fSet.BoolVar(&help, "help", false, "")
	fSet.BoolVar(&help, "h", false, "")

	fSet.Usage = func() {
		err = errors.New("usage error")
		printedUsage = true

		fmt.Fprintln(os.Stderr, `Usage:
 ubuntu-proxy-manager [options]

Start proxy manager service

Options:
 -d, --debug     enable debug logging
 -v, --version   print version and exit
 -h, --help      print this message and exit

ubuntu-proxy-manager is a proxy manager for Ubuntu Desktop. This program is not
intended to be run by hand, rather by a D-Bus activated systemd service.

When activated, it will listen for D-Bus calls to set the system proxy
configuration (APT, environment, GSettings). The program will exit if no D-Bus
call is received shortly after activation.

The program does not take any arguments.`)
	}

	parseErr := fSet.Parse(os.Args[1:])
	if len(fSet.Args()) > 0 || parseErr != nil {
		fSet.Usage()
		return true, errors.New("usage error")
	}

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	if version {
		fmt.Printf("ubuntu-proxy-manager\t%s\n", app.Version)
		return true, nil
	}

	if help {
		fSet.Usage()
		return true, nil
	}

	return printedUsage, err
}
