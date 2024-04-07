package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/simulot/immich-go/cmd"
	"github.com/simulot/immich-go/cmd/upload"
	"github.com/simulot/immich-go/logger"
	"github.com/simulot/immich-go/ui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var err error
	fmt.Println()
	fmt.Println(ui.Banner.ToString(fmt.Sprintf("%s, commit %s, built at %s\n", version, commit, date)))

	// Create a context with cancel function to gracefully handle Ctrl+C events
	ctx, cancel := context.WithCancel(context.Background())

	// Handle Ctrl+C signal (SIGINT)
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)

	go func() {
		<-signalChannel
		fmt.Println("\nCtrl+C received. Shutting down...")
		cancel() // Cancel the context when Ctrl+C is received
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	default:
		err = Run(ctx)
	}
	if err != nil {
		os.Exit(1)
	}
}

func Run(ctx context.Context) error {
	log := logger.NewLogger("OK", true)

	app := cmd.SharedFlags{}
	defer app.Close()

	fs := flag.NewFlagSet("main", flag.ExitOnError)
	app.InitSharedFlags(ui.Banner.ToString(fmt.Sprintf("%s, commit %s, built at %s\n", version, commit, date)))
	app.Log = log
	app.SetFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	if len(fs.Args()) == 0 {
		err = errors.Join(err, errors.New("missing command upload|duplicate|stack|tool"))
	}

	if err != nil {
		return err
	}

	cmd := fs.Args()[0]
	switch cmd {
	case "upload":
		err = upload.UploadCommand(ctx, &app, fs.Args()[1:])
		/*
			case "duplicate":
				err = duplicate.DuplicateCommand(ctx, &app, fs.Args()[1:])
			case "metadata":
				err = metadata.MetadataCommand(ctx, &app, fs.Args()[1:])
			case "stack":
				err = stack.NewStackCommand(ctx, &app, fs.Args()[1:])
			case "tool":
				err = tool.CommandTool(ctx, &app, fs.Args()[1:])
		*/
	default:
		err = fmt.Errorf("unknown command: %q", cmd)
	}

	if err != nil {
		log.Error(err)
	}
	return err
}
