package steps

import (
	"context"
	"fmt"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	config2 "github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/config"
	"github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/proxy"
	"github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/vnc"
	"github.com/xenserver/packer-builder-xenserver/builder/xenserver/common/xen"
	"net"
	"strconv"
)

type StepGetVNCPort struct {
	forwarding proxy.ProxyForwarding
}

func (self *StepGetVNCPort) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	xenClient := state.Get("client").(*xen.Connection)
	xenProxy := state.Get("xen_proxy").(proxy.XenProxy)
	config := state.Get("commonconfig").(config2.CommonConfig)

	if config.VNCConfig.DisableVNC {
		return multistep.ActionContinue
	}

	ui.Say("Step: forward the instances VNC")

	location, err := vnc.GetVNCConsoleLocation(state)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	target, err := vnc.GetTcpAddressFromURL(location)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	host, portText, err := net.SplitHostPort(target)

	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	port, err := strconv.Atoi(portText)

	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	self.forwarding = xenProxy.CreateWrapperForwarding(host, port, func(rawConn net.Conn) (net.Conn, error) {
		return vnc.InitializeVNCConnection(location, string(xenClient.GetSessionRef()), rawConn)
	})

	err = self.forwarding.Start()

	if err != nil {
		self.forwarding.Close()
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	vncUrl := net.JoinHostPort(self.forwarding.GetServiceHost(), strconv.Itoa(self.forwarding.GetServicePort()))
	ui.Say(fmt.Sprintf("VNC available on vnc://%s", vncUrl))

	return multistep.ActionContinue
}

func (self *StepGetVNCPort) Cleanup(state multistep.StateBag) {
	if self.forwarding != nil {
		self.forwarding.Close()
	}
}