package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/region"
)

func applyNetworkPrefs(cfg *identity.Config) {
	if cfg == nil {
		return
	}
	netutil.SetEchoURLOverrides(cfg.IPEchoCN, cfg.IPEchoIntl)
	region.SetGeoURLOverride(cfg.GeoURL)
}

func cmdConfig() {
	if len(os.Args) < 2 {
		configShow()
		return
	}
	switch os.Args[1] {
	case "show":
		configShow()
	case "set-ip-echo":
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		configSetIPEcho()
	case "set-geo-url":
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		configSetGeoURL()
	case "reset-endpoints":
		configResetEndpoints()
	case "help", "-h", "--help":
		configUsage()
	default:
		fmt.Fprint(os.Stderr, i18n.T("config.unknown", os.Args[1]))
		configUsage()
		os.Exit(2)
	}
}

func configUsage() {
	fmt.Fprint(os.Stderr, i18n.T("config.usage",
		strings.Join(netutil.DefaultEchoURLsCN(), ", "),
		strings.Join(netutil.DefaultEchoURLsIntl(), ", "),
		region.DefaultGeoURL))
}

func configShow() {
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	applyNetworkPrefs(cfg)
	cn, intl := netutil.ActiveEchoURLs()
	fmt.Println(i18n.T("config.endpoints_effective"))
	fmt.Printf("  ip_echo_cn:   %s\n", strings.Join(cn, ", "))
	fmt.Printf("  ip_echo_intl: %s\n", strings.Join(intl, ", "))
	fmt.Printf("  geo_url:      %s\n", region.GeoURL())
	fmt.Println()
	fmt.Println(i18n.T("config.overrides"))
	def := i18n.T("config.default")
	if len(cfg.IPEchoCN) == 0 {
		fmt.Printf("  ip_echo_cn:   %s\n", def)
	} else {
		fmt.Printf("  ip_echo_cn:   %s\n", strings.Join(cfg.IPEchoCN, ", "))
	}
	if len(cfg.IPEchoIntl) == 0 {
		fmt.Printf("  ip_echo_intl: %s\n", def)
	} else {
		fmt.Printf("  ip_echo_intl: %s\n", strings.Join(cfg.IPEchoIntl, ", "))
	}
	if cfg.GeoURL == "" {
		fmt.Printf("  geo_url:      %s\n", def)
	} else {
		fmt.Printf("  geo_url:      %s\n", cfg.GeoURL)
	}
	if o := region.Override(); o != "" {
		fmt.Print(i18n.T("config.region_forced", o))
	}
}

func configSetIPEcho() {
	cnFlag := flag.String("cn", "", "comma-separated mainland echo URLs")
	intlFlag := flag.String("intl", "", "comma-separated international echo URLs")
	flag.Parse()
	if strings.TrimSpace(*cnFlag) == "" && strings.TrimSpace(*intlFlag) == "" {
		log.Fatal(i18n.T("config.need_cn_or_intl"))
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if v := strings.TrimSpace(*cnFlag); v != "" {
		list := netutil.ParseEchoURLList(v)
		if len(list) == 0 {
			log.Fatal(i18n.T("config.empty_cn"))
		}
		cfg.IPEchoCN = list
	}
	if v := strings.TrimSpace(*intlFlag); v != "" {
		list := netutil.ParseEchoURLList(v)
		if len(list) == 0 {
			log.Fatal(i18n.T("config.empty_intl"))
		}
		cfg.IPEchoIntl = list
	}
	if err := identity.SaveConfig(cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Println(i18n.T("config.saved_ip_echo"))
	configShow()
}

func configSetGeoURL() {
	flag.Parse()
	url := ""
	if flag.NArg() > 0 {
		url = strings.TrimSpace(flag.Arg(0))
	}
	if url == "" {
		log.Fatal(i18n.T("config.geo_usage"))
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	cfg.GeoURL = url
	if err := identity.SaveConfig(cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Println(i18n.T("config.saved_geo"))
	configShow()
}

func configResetEndpoints() {
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	cfg.IPEchoCN = nil
	cfg.IPEchoIntl = nil
	cfg.GeoURL = ""
	if err := identity.SaveConfig(cfg); err != nil {
		log.Fatal(err)
	}
	netutil.SetEchoURLOverrides(nil, nil)
	region.SetGeoURLOverride("")
	fmt.Println(i18n.T("config.reset_ok"))
	configShow()
}
