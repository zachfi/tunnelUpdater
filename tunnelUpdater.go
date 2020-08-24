package main

import (
	"sync"

	tunnelbroker "github.com/xaque208/go-tunnelbroker"

	"flag"
	"fmt"
	"strings"

	"github.com/scottdware/go-junos"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Status struct {
	ExternalAddress string
	Router          TunnelStatus
	Provider        TunnelStatus
	ProviderID      int
}

type TunnelStatus struct {
	NearSideIP string
	FarSideIP  string
}

type JuniperDevice struct {
	HostName   string
	UserName   string
	KeyFile    string
	PassPhrase string
}

func getStatus(tunnelBrokerClient tunnelbroker.Client, juniperDevice JuniperDevice) (*Status, error) {
	var status Status
	var wg sync.WaitGroup

	externalInterface := viper.GetString("junos.externalInterface")
	tunnelInterface := viper.GetString("junos.tunnelInterface")

	wg.Add(1)
	go func() {
		defer wg.Done()

		log.Debug("Reading tunnelbroker status")
		info, err := tunnelBrokerClient.TunnelInfo()
		if err != nil {
			log.Fatal(err)
		}

		log.WithFields(log.Fields{
			"tunnel_info":  info,
			"tunnel_count": len(info.Tunnels),
		}).Debug("tunnel broker status")

		if len(info.Tunnels) > 0 {
			status.ProviderID = info.Tunnels[0].ID
			status.Provider.NearSideIP = info.Tunnels[0].ClientV4
			status.Provider.FarSideIP = info.Tunnels[0].ServerV4
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.WithFields(log.Fields{
			"external_interface": externalInterface,
			"tunnel_interface":   tunnelInterface,
		}).Debug("reading interface config")

		session := juniperDevice.Session()

		views, err := session.View("interface")
		if err != nil {
			log.Fatal(err)
		}

		for _, physicalInterface := range views.Interface.Entries {
			for _, logicalInterface := range physicalInterface.LogicalInterfaces {
				log.WithFields(log.Fields{
					"name":             logicalInterface.Name,
					"address_families": logicalInterface.AddressFamilies,
				}).Trace("logical_interface")

				if logicalInterface.Name == externalInterface {
					for _, af := range logicalInterface.AddressFamilies {
						log.WithFields(log.Fields{
							"name":       af.Name,
							"cidr":       af.CIDR,
							"ip_address": af.IPAddress,
						}).Debug("address_family")

						if af.Name == "inet" {
							status.ExternalAddress = af.IPAddress
						}
					}

				}

				if logicalInterface.Name == tunnelInterface {
					parts := strings.Split(logicalInterface.LinkAddress, ":")

					status.Router.FarSideIP = parts[0]
					status.Router.NearSideIP = parts[1]
				}
			}
		}
	}()

	wg.Wait()

	if status.Router != status.Provider {
		log.WithFields(log.Fields{
			"provider_near":    status.Provider.NearSideIP,
			"provider_far":     status.Provider.FarSideIP,
			"router_near":      status.Router.NearSideIP,
			"router_far":       status.Router.FarSideIP,
			"external_address": status.ExternalAddress,
		}).Warn("status does not agree")
	}

	return &status, nil
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

func (j *JuniperDevice) SetTunnelConfigSource(tunnelInterface, address string) {
	session := j.Session()

	setConfig := fmt.Sprintf("set interfaces %s tunnel source %s", tunnelInterface, address)
	err := session.Config(setConfig, "set", true)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	var verbose bool
	var configPath string
	flag.BoolVar(&verbose, "v", false, "Increase verbosity")
	flag.StringVar(&configPath, "c", "", "Directory to look for configuration")

	flag.Parse()

	viper.SetConfigName("tunnelUpdater")
	viper.AddConfigPath(".")
	if configPath != "" {
		viper.AddConfigPath(configPath)
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}

	tunnelBrokerClient := tunnelbroker.Client{
		Username: viper.GetString("tunnelbroker.username"),
		Password: viper.GetString("tunnelbroker.password"),
	}

	juniperDevice := JuniperDevice{
		HostName:   viper.GetString("junos.hostname"),
		UserName:   viper.GetString("junos.username"),
		KeyFile:    viper.GetString("junos.keyfile"),
		PassPhrase: viper.GetString("junos.passphrase"),
	}

	tunnelInterface := viper.GetString("junos.tunnelInterface")

	status, err := getStatus(tunnelBrokerClient, juniperDevice)
	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"status": status,
	}).Debug("status config")

	if status.Provider.NearSideIP != status.ExternalAddress {
		log.Infof("Setting TunnelBroker ClientV4 address to %s", status.ExternalAddress)
		tunnelBrokerClient.UpdateTunnel(status.ProviderID, status.ExternalAddress)
	}

	if status.Router.NearSideIP != status.ExternalAddress {
		log.Infof("Setting tunnel interface source to external address %s", status.ExternalAddress)
		juniperDevice.SetTunnelConfigSource(
			tunnelInterface,
			status.ExternalAddress,
		)

	}

}
