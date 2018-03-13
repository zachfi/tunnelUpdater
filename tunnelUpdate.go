package main

import (
	tunnelbroker "github.com/xaque208/go-tunnelbroker"

	"fmt"
	"github.com/scottdware/go-junos"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
	"log"
)

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func Juniper() {
	sshConfig := &ssh.ClientConfig{
		User: "zach",
		Auth: []ssh.AuthMethod{PublicKeyFile("/home/zach/.ssh/id_ed25519")},
	}
	jnpr, err := junos.NewSession("fw01.l.larch.space", sshConfig)
}

func main() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}

	tb_client := tunnelbroker.Client{
		Username: viper.GetString("tunnelbroker.username"),
		Password: viper.GetString("tunnelbroker.password"),
	}

	info, err := tb_client.TunnelInfo()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v", info)

	Juniper()
}
