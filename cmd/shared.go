package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/simulot/immich-go/helpers/configuration"
	"github.com/simulot/immich-go/helpers/myflag"
	"github.com/simulot/immich-go/helpers/tzone"
	"github.com/simulot/immich-go/immich"
)

// SharedFlags collect all parameters that are common to all commands
type SharedFlags struct {
	ConfigurationFile string // Path to the configuration file to use
	Server            string // Immich server address (http://<your-ip>:2283/api or https://<your-domain>/api)
	API               string // Immich api endpoint (http://container_ip:3301)
	Key               string // API Key
	DeviceUUID        string // Set a device UUID
	APITrace          bool   // Enable API call traces
	NoLogColors       bool   // Disable log colors
	LogLevel          string // Indicate the log level
	Debug             bool   // Enable the debug mode
	TimeZone          string // Override default TZ
	SkipSSL           bool   // Skip SSL Verification

	Immich  immich.ImmichInterface // Immich client
	LogFile string                 // Log file
	out     io.Writer              // the log writer
	Log     *log.Logger
	Banner  string
}

func (app *SharedFlags) InitSharedFlags(banner string) {
	app.ConfigurationFile = configuration.DefaultFile()
	app.NoLogColors = runtime.GOOS == "windows"
	app.APITrace = false
	app.Debug = false
	app.SkipSSL = false
	app.LogFile = "./immich-go-" + time.Now().Format(time.DateTime) + ".log"
	app.Banner = banner
}

// SetFlag add common flags to a flagset
func (app *SharedFlags) SetFlags(fs *flag.FlagSet) {
	fs.StringVar(&app.ConfigurationFile, "use-configuration", app.ConfigurationFile, "Specifies the configuration to use")
	fs.StringVar(&app.Server, "server", app.Server, "Immich server address (http://<your-ip>:2283 or https://<your-domain>)")
	fs.StringVar(&app.API, "api", "", "Immich api endpoint (http://container_ip:3301)")
	fs.StringVar(&app.Key, "key", app.Key, "API Key")
	fs.StringVar(&app.DeviceUUID, "device-uuid", app.DeviceUUID, "Set a device UUID")
	fs.BoolFunc("no-colors-log", "Disable colors on logs", myflag.BoolFlagFn(&app.NoLogColors, app.NoLogColors))
	fs.StringVar(&app.LogLevel, "log-level", app.LogLevel, "Log level (Error|Warning|OK|Info), default OK")
	fs.StringVar(&app.LogFile, "log-file", app.LogFile, "Write log messages into the file")
	fs.BoolFunc("api-trace", "enable api call traces", myflag.BoolFlagFn(&app.APITrace, app.APITrace))
	fs.BoolFunc("debug", "enable debug messages", myflag.BoolFlagFn(&app.Debug, app.Debug))
	fs.StringVar(&app.TimeZone, "time-zone", app.TimeZone, "Override the system time zone")
	fs.BoolFunc("skip-verify-ssl", "Skip SSL verification", myflag.BoolFlagFn(&app.SkipSSL, app.SkipSSL))
}

func (app *SharedFlags) Start(ctx context.Context) error {
	var joinedErr error
	if app.Server != "" {
		app.Server = strings.TrimSuffix(app.Server, "/")
	}
	if app.TimeZone != "" {
		_, err := tzone.SetLocal(app.TimeZone)
		joinedErr = errors.Join(joinedErr, err)
	}

	var err error

	if app.out == nil {
		app.out, err = os.OpenFile(app.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o664)
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
		} else {
			app.Log.SetReportTimestamp(true)
			app.Log.SetOutput(app.out)
			app.Log.Print("\n" + app.Banner)
		}
	}
	// at this point, exits if there is an error
	if joinedErr != nil {
		return joinedErr
	}

	// If the client isn't yet initialized
	if app.Immich == nil {
		if app.Server == "" && app.API == "" && app.Key == "" {
			conf, err := configuration.Read(app.ConfigurationFile)
			if err == nil {
				app.Server = conf.ServerURL
				app.API = conf.APIURL
				app.Key = conf.APIKey
			}
		}

		switch {
		case app.Server == "" && app.API == "":
			joinedErr = errors.Join(joinedErr, errors.New("missing -server, Immich server address (http://<your-ip>:2283 or https://<your-domain>)"))
		case app.Server != "" && app.API != "":
			joinedErr = errors.Join(joinedErr, errors.New("give either the -server or the -api option"))
		}
		if app.Key == "" {
			joinedErr = errors.Join(joinedErr, errors.New("missing -key"))
			return joinedErr
		}

		// Connection details are saved into the configuration file
		conf := configuration.Configuration{
			ServerURL: app.Server,
			APIKey:    app.Key,
			APIURL:    app.API,
		}
		err = conf.Write(app.ConfigurationFile)
		if err != nil {
			err = fmt.Errorf("can't write into the configuration file: %w", err)
			joinedErr = errors.Join(joinedErr, err)
			return joinedErr
		}

		app.Immich, err = immich.NewImmichClient(app.Server, app.Key, app.SkipSSL)
		if err != nil {
			return err
		}
		if app.API != "" {
			app.Immich.SetEndPoint(app.API)
		}
		if app.APITrace {
			app.Immich.EnableAppTrace(true)
		}
		if app.DeviceUUID != "" {
			app.Immich.SetDeviceUUID(app.DeviceUUID)
		}

		err = app.Immich.PingServer(ctx)
		if err != nil {
			return err
		}
		app.Log.Print("Server status: OK")

		user, err := app.Immich.ValidateConnection(ctx)
		if err != nil {
			return err
		}
		app.Log.Printf("Connected, user: %s", user.Email)
	}
	return nil
}

func (app *SharedFlags) Close() error {
	if closer, ok := app.out.(io.Closer); ok {
		fmt.Println("Consult the log file for details:", app.LogFile)
		return closer.Close()
	}
	return nil
}
