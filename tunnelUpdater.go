package main

import (
	tunnelbroker "github.com/xaque208/go-tunnelbroker"

	"fmt"
	"github.com/scottdware/go-junos"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
)

type JuniperDevice struct {
	HostName   string
	UserName   string
	KeyFile    string
	PassPhrase string
}

type junosTunnel struct {
	Source      string
	Destination string
}

func (j *JuniperDevice) Session() *junos.Junos {
	auth := &junos.AuthMethod{
		Username:   j.UserName,
		PrivateKey: j.KeyFile,
	}

	session, err := junos.NewSession(j.HostName, auth)
	if err != nil {
		log.Fatal(err)
	}

	return session
}

func (j *JuniperDevice) InterfaceConfigs(externalInterface, tunnelInterface string) (string, junosTunnel) {
	session := j.Session()

	views, err := session.View("interface")
	if err != nil {
		log.Fatal(err)
	}

	var externalAddress string
	var tunnelConfig junosTunnel

	for _, pInterface := range views.Interface.Entries {
		for _, lInterface := range pInterface.LogicalInterfaces {
			if lInterface.Name == externalInterface {
				externalAddress = lInterface.IPAddress
			}

			if lInterface.Name == tunnelInterface {
				parts := strings.Split(lInterface.LinkAddress, ":")

				tunnelConfig = junosTunnel{
					Destination: parts[0],
					Source:      parts[1],
				}
			}
		}
	}

	return externalAddress, tunnelConfig
}

func (j *JuniperDevice) SetTunnelConfigSource(tunnelInterface, address string) {
	session := j.Session()

	setConfig := fmt.Sprintf("set interfaces %s tunnel source %s", tunnelInterface, address)
	err := session.Config(setConfig, "set", true)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	viper.SetConfigName("tunnelUpdater")
	viper.AddConfigPath(".")

	log.SetLevel(log.DebugLevel)

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}

	tb_client := tunnelbroker.Client{
		Username: viper.GetString("tunnelbroker.username"),
		Password: viper.GetString("tunnelbroker.password"),
	}

	j_device := JuniperDevice{
		HostName:   viper.GetString("junos.hostname"),
		UserName:   viper.GetString("junos.username"),
		KeyFile:    viper.GetString("junos.keyfile"),
		PassPhrase: viper.GetString("junos.passphrase"),
	}

	externalInterface := viper.GetString("junos.externalInterface")
	tunnelInterface := viper.GetString("junos.tunnelInterface")

	log.Debug("Reading tunnelbroker status")
	tunnelInfo, err := tb_client.TunnelInfo()
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Reading router interface status")
	externalAddress, tunnelConfig := j_device.InterfaceConfigs(
		externalInterface,
		tunnelInterface,
	)

	if tunnelInfo.Tunnels[0].ClientV4 != externalAddress {
		log.Infof("Setting TunnelBroker ClientV4 address to %s", externalAddress)
		tb_client.UpdateTunnel(tunnelInfo.Tunnels[0].Id, externalAddress)
	}

	if tunnelConfig.Source != externalAddress {
		log.Infof("Setting tunnell interface source to external address %s", externalAddress)
		j_device.SetTunnelConfigSource(
			tunnelInterface,
			externalAddress,
		)
	}

}
