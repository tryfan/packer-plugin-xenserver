package steps

import (
	"context"
	"fmt"
	config2 "github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/config"
	"github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/util"
	"github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/xen"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepWaitForIP struct {
	VmCleanup
	Chan    <-chan string
	Timeout time.Duration
}

func (self *StepWaitForIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	c := state.Get("client").(*xen.Connection)
	config := state.Get("commonconfig").(config2.CommonConfig)

	// Respect static configuration
	if config.Comm.Host() != "" {
		state.Put("instance_ssh_address", config.Comm.Host())
		return multistep.ActionContinue
	}

	ui.Say("Step: Wait for VM's IP to become known to us.")

	uuid := state.Get("instance_uuid").(string)
	instance, err := c.GetClient().VM.GetByUUID(c.GetSessionRef(), uuid)
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get VM from UUID '%s': %s", uuid, err.Error()))
		return multistep.ActionHalt
	}

	var ip string
	err = util.InterruptibleWait{
		Timeout:           self.Timeout,
		PredicateInterval: 5 * time.Second,
		Predicate: func() (result bool, err error) {

			if config.IPGetter == "auto" || config.IPGetter == "http" {

				// Snoop IP from HTTP fetch
				select {
				case ip = <-self.Chan:
					ui.Message(fmt.Sprintf("Got IP '%s' from HTTP request", ip))
					return true, nil
				default:
				}

			}

			if config.IPGetter == "auto" || config.IPGetter == "tools" {

				// Look for PV IP
				m, err := c.GetClient().VM.GetGuestMetrics(c.GetSessionRef(), instance)
				if err != nil {
					return false, err
				}
				if m != "" {
					metrics, err := c.GetClient().VMGuestMetrics.GetRecord(c.GetSessionRef(), m)
					if err != nil {
						return false, err
					}
					networks := metrics.Networks
					if ip, ok := networks["0/ip"]; ok {
						if ip != "" {
							ui.Message(fmt.Sprintf("Got IP '%s' from XenServer tools", ip))
							return true, nil
						}
					}
				}

			}

			return false, nil
		},
	}.Wait(state)
	if err != nil {
		ui.Error(fmt.Sprintf("Could not get IP address of VM: %s", err.Error()))
		// @todo: give advice on what went wrong (no HTTP server? no PV drivers?)
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("Got IP address '%s'", ip))
	state.Put("instance_ssh_address", ip)

	return multistep.ActionContinue
}

func InstanceCommIP(state multistep.StateBag) (string, error) {
	ip := state.Get("instance_ssh_address").(string)
	return ip, nil
}

func InstanceCommPort(state multistep.StateBag) (int, error) {
	config := state.Get("commonconfig").(config2.CommonConfig)
	return config.Comm.Port(), nil
}