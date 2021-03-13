package main

import (
	"fmt"

	"github.com/mikesmitty/mdns-mesh/mdns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	switch {
	case viper.GetBool("trace"):
		log.SetLevel(log.TraceLevel)
	case viper.GetBool("debug"):
		log.SetLevel(log.DebugLevel)
	case viper.GetBool("verbose"):
		log.SetLevel(log.InfoLevel)
	case viper.GetBool("quiet"):
		log.SetLevel(log.FatalLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}

	queue := fmt.Sprintf("nats://%s:%d", viper.GetString("address"), viper.GetInt("port"))

	config := mdns.Config{
		FilterTTL:   viper.GetInt("filter-ttl"),
		HighPort:    viper.GetBool("high-port"),
		ListenIP:    viper.GetString("listen-ip"),
		Monitor:     viper.GetStringSlice("monitor"),
		PortFilters: viper.GetStringSlice("port-filters"),
		Queue:       queue,
		UniqueID:    viper.GetString("unique-id"),
	}

	err := mdns.StartServer(config)
	if err != nil {
		log.Fatalf("Error starting listener: %v", err)
	}
}

func init() {
	pflag.StringP("config", "c", "", "config file (default is $HOME/mdns-mesh.yaml)")
	pflag.StringP("address", "a", "", "NATS queue address")
	pflag.IntP("port", "p", 4222, "NATS queue port")
	pflag.StringSliceP("monitor", "m", nil, "network interface(s) on which to send/receive mDNS traffic")
	pflag.IntP("filter-ttl", "t", 1, "TTL used to mark outgoing packets to prevent broadcast loops")
	pflag.StringP("listen-ip", "l", "", "ip address from which to listen and send broadcasts")
	pflag.String("unique-id", "", "sender id used to filter out each client's own traffic from the queue")
	pflag.Bool("high-port", false, "send broadcasts from separate socket with a high port")
	pflag.StringSlice("port-filters", nil, "regex filters to send traffic to high or low source ports")

	pflag.BoolP("quiet", "q", false, "enable verbose mode")
	pflag.BoolP("verbose", "v", false, "enable verbose mode")
	pflag.Bool("debug", false, "enable debug mode")
	pflag.Bool("trace", false, "enable trace mode")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	cfgFile := viper.GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigName("mdns-mesh")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
