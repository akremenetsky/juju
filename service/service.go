// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/utils"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

var (
	logger   = loggo.GetLogger("juju.service")
	renderer = shell.BashRenderer{}
)

// These are the names of the init systems recognized by juju.
const (
	InitSystemSystemd = "systemd"
	InitSystemUpstart = "upstart"
	InitSystemWindows = "windows"
	SystemdDataDir    = "/lib/systemd/system"
)

// linuxInitSystems lists the names of the init systems that juju might
// find on a linux host.
var linuxInitSystems = []string{
	InitSystemSystemd,
	InitSystemUpstart,
}

// ServiceActions represents the actions that may be requested for
// an init system service.
type ServiceActions interface {
	// Start will try to start the service.
	Start() error

	// Stop will try to stop the service.
	Stop() error

	// Install installs a service.
	Install() error

	// Remove will remove the service.
	Remove() error
}

// Service represents a service in the init system running on a host.
type Service interface {
	ServiceActions

	// Name returns the service's name.
	Name() string

	// Conf returns the service's conf data.
	Conf() common.Conf

	// Running returns a boolean value that denotes
	// whether or not the service is running.
	Running() (bool, error)

	// Exists returns whether the service configuration exists in the
	// init directory with the same content that this Service would have
	// if installed.
	Exists() (bool, error)

	// Installed will return a boolean value that denotes
	// whether or not the service is installed.
	Installed() (bool, error)

	// TODO(ericsnow) Move all the commands into a separate interface.

	// InstallCommands returns the list of commands to run on a
	// (remote) host to install the service.
	InstallCommands() ([]string, error)

	// StartCommands returns the list of commands to run on a
	// (remote) host to start the service.
	StartCommands() ([]string, error)
}

// RestartableService is a service that directly supports restarting.
type RestartableService interface {
	// Restart restarts the service.
	Restart() error
}

type UpgradableService interface {
	// WriteService write the service conf data, if the service is
	// running add links to allow for manual and automatic start
	// of the service.
	WriteService() error
}

// TODO(ericsnow) bug #1426458
// Eliminate the need to pass an empty conf for most service methods
// and several helper functions.

// NewService returns a new Service based on the provided info.
var NewService = func(name string, conf common.Conf, series string) (Service, error) {
	if name == "" {
		return nil, errors.New("missing name")
	}

	initSystem, err := versionInitSystem(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newService(name, conf, initSystem, series)
}

// this needs to be stubbed out in some tests
func newService(name string, conf common.Conf, initSystem, series string) (Service, error) {
	switch initSystem {
	case InitSystemWindows:
		svc, err := windows.NewService(name, conf)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to wrap service %q", name)
		}
		return svc, nil
	case InitSystemUpstart:
		return upstart.NewService(name, conf), nil
	case InitSystemSystemd:
		dataDir, err := paths.DataDir(series)
		if err != nil {
			return nil, err
		}
		svc, err := systemd.NewService(
			name,
			conf,
			SystemdDataDir,
			systemd.NewDBusAPI,
			renderer.Join(dataDir, "init"),
		)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to wrap service %q", name)
		}
		return svc, nil
	default:
		return nil, errors.NotFoundf("init system %q", initSystem)
	}
}

// ListServices lists all installed services on the running system
var ListServices = func() ([]string, error) {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	initName, err := VersionInitSystem(hostSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch initName {
	case InitSystemWindows:
		services, err := windows.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	case InitSystemUpstart:
		services, err := upstart.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	case InitSystemSystemd:
		services, err := systemd.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
}

// ListServicesScript returns the commands that should be run to get
// a list of service names on a host.
func ListServicesScript() string {
	commands := []string{
		"init_system=$(" + DiscoverInitSystemScript() + ")",
		// If the init system is not identified then the script will
		// "exit 1". This is correct since the script should fail if no
		// init system can be identified.
		newShellSelectCommand("init_system", "exit 1", listServicesCommand),
	}
	return strings.Join(commands, "\n")
}

func listServicesCommand(initSystem string) (string, bool) {
	switch initSystem {
	case InitSystemWindows:
		return windows.ListCommand(), true
	case InitSystemUpstart:
		return upstart.ListCommand(), true
	case InitSystemSystemd:
		return systemd.ListCommand(), true
	default:
		return "", false
	}
}

// installStartRetryAttempts defines how much InstallAndStart retries
// upon Start failures.
//
// TODO(katco): 2016-08-09: lp:1611427
var installStartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// InstallAndStart installs the provided service and tries starting it.
// The first few Start failures are ignored.
func InstallAndStart(svc ServiceActions) error {
	logger.Infof("Installing and starting service %+v", svc)
	if err := svc.Install(); err != nil {
		return errors.Trace(err)
	}

	// For various reasons the init system may take a short time to
	// realise that the service has been installed.
	var err error
	for attempt := installStartRetryAttempts.Start(); attempt.Next(); {
		if err != nil {
			logger.Errorf("retrying start request (%v)", errors.Cause(err))
		}
		// we attempt restart if the service is running in case daemon parameters
		// have changed, if its not running a regular start will happen.
		if err = restartOrStart(svc); err == nil {
			break
		}
	}
	return errors.Trace(err)
}

// discoverService is patched out during some tests.
var discoverService = func(name string) (Service, error) {
	return DiscoverService(name, common.Conf{})
}

// TODO(ericsnow) Add one-off helpers for Start and Stop too?

// Restart restarts the named service.
func Restart(name string) error {
	svc, err := discoverService(name)
	if err != nil {
		return errors.Annotatef(err, "failed to find service %q", name)
	}
	if err := restart(svc); err != nil {
		return errors.Annotatef(err, "failed to restart service %q", name)
	}
	return nil
}

func restartOrStart(svc ServiceActions) error {
	// Explicitly omit Restart as it is not properly supported on trusty.
	// Otherwise explicitly stop and start the service.
	if err := svc.Stop(); err != nil {
		logger.Errorf("could not stop service: %v", err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil

}

func restart(svc Service) error {
	// Use the Restart method, if there is one.
	if svc, ok := svc.(RestartableService); ok {
		if err := svc.Restart(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	// Otherwise explicitly stop and start the service.
	if err := svc.Stop(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
