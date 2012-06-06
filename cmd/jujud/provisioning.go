package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/state"
	"launchpad.net/tomb"

	// register providers
	_ "launchpad.net/juju/go/environs/dummy"
	_ "launchpad.net/juju/go/environs/ec2"
)

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	Conf AgentConf
}

// Info returns usage information for the command.
func (a *ProvisioningAgent) Info() *cmd.Info {
	return &cmd.Info{"provisioning", "", "run a juju provisioning agent", ""}
}

// Init initializes the command for running.
func (a *ProvisioningAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return a.Conf.checkArgs(f.Args())
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	// TODO(dfc) place the logic in a loop with a suitable delay
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	p := NewProvisioner(st)
	return p.Wait()
}

type Provisioner struct {
	st      *state.State
	environ environs.Environ
	tomb    tomb.Tomb

	environWatcher  *state.ConfigWatcher
	machinesWatcher *state.MachinesWatcher
}

// NewProvisioner returns a Provisioner.
func NewProvisioner(st *state.State) *Provisioner {
	p := &Provisioner{
		st: st,
	}
	go p.loop()
	return p
}

func (p *Provisioner) loop() {
	defer p.tomb.Done()

	p.environWatcher = p.st.WatchEnvironConfig()
	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case config, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			var err error
			p.environ, err = environs.NewEnviron(config.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			log.Printf("provisioner loaded new environment configuration")
			p.innerLoop()
		}
	}
}

func (p *Provisioner) innerLoop() {
	p.machinesWatcher = p.st.WatchMachines()
	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case change, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			config, err := environs.NewConfig(change.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			p.environ.SetConfig(config)
			log.Printf("provisioner loaded new environment configuration")
		case machines, ok := <-p.machinesWatcher.Changes():
			if !ok {
				err := p.machinesWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			p.processMachines(machines)
		}
	}
}

// Wait waits for the Provisioner to exit.
func (p *Provisioner) Wait() error {
	return p.tomb.Wait()
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

func (p *Provisioner) processMachines(changes *state.MachinesChange) {}
