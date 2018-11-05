package main

import (
	"encoding/json"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	namespace = "proktologe"
)

type Ports struct {
	Address string `json:"address"`
	Open    []int  `json:"open"`
}

type Exporter struct {
	proktologe string
	timeout time.Duration

	up *prometheus.Desc
	open *prometheus.Desc
}

func NewExporter(address string, timeout time.Duration) *Exporter {
	return &Exporter{
		proktologe: address,
		timeout: timeout,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Could the proktologe server be reached",
			[]string{},
			prometheus.Labels{},
			),
		open: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "open"),
			"Open ports on external interface",
			[]string{},
			prometheus.Labels{},
			),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.open
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, float64(1))
	//status := 1
	external, err := externalIP()
	if err != nil {
		log.Error(err)
		return
	}
	if len(external) > 0 {
		log.Infof("External IP address is: %s", external)

		data, err := e.scanMy(external)
		if err != nil {
			log.Error(err)
			return
		}

		for _, v := range data.Open {
			labels := prometheus.Labels{
				"address": data.Address,
				"port":    strconv.Itoa(v),
			}
			e.open = prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "open"),
				"Open ports on external interface",
				[]string{},
				labels,
			)
			ch <- prometheus.MustNewConstMetric(e.open, prometheus.GaugeValue, float64(1))
		}
	}
}

func (e *Exporter) scanMy(ip string) (*Ports, error) {
	var data *Ports

	resp, err := http.Get(e.proktologe + "/scan/" + ip)
	if err != nil {
		log.Error(err)
		return data, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return data, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return data, err
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return data, err
	}
	return data, err
}

func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if !IsPublicIP(ip) {
				continue
			}
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("Public IP not found")
}

func IsPublicIP(IP net.IP) bool {
	if IP.IsLoopback() || IP.IsLinkLocalMulticast() || IP.IsLinkLocalUnicast() {
		return false
	}
	if ip4 := IP.To4(); ip4 != nil {
		switch true {
		case ip4[0] == 10:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		default:
			return true
		}
	}
	return false
}

func main() {
	var (
		address       = kingpin.Flag("proktologe.address", "Proktologe server address.").Default("http://127.0.0.1").String()
		timeout       = kingpin.Flag("proktologe.timeout", "Proktologe connect timeout.").Default("60s").Duration()
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9247").String()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()

	)
	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("proktologe_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	log.Infoln("Starting proktologe_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	prometheus.MustRegister(NewExporter(*address, *timeout))

	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
      <head><title>Proktologe Exporter</title></head>
      <body>
      <h1>Proktologe Exporter</h1>
      <p><a href='` + *metricsPath + `'>Metrics</a></p>
      </body>
      </html>`))
	})
	log.Infoln("Starting HTTP server on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}