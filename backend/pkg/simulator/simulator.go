package simulator

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

type MultiDeviceSimulator struct {
	mu           sync.Mutex
	running      bool
	hikServer    *http.Server
	dahuaServer  *http.Server
	onvifServer  *http.Server
	rtspListener net.Listener

	// Simulated state
	hikIP       string
	hikDHCP     bool
	hikBright   int
	hikContrast int

	dahuaIP     string
	dahuaDHCP   bool
	dahuaBright int
	dahuaFormat string // TVI/CVI/AHD/CVBS
}

func NewSimulator() *MultiDeviceSimulator {
	return &MultiDeviceSimulator{
		hikIP:       "127.0.0.1",
		hikDHCP:     false,
		hikBright:   55,
		hikContrast: 50,

		dahuaIP:     "127.0.0.1",
		dahuaDHCP:   false,
		dahuaBright: 50,
		dahuaFormat: "CVI",
	}
}

func (s *MultiDeviceSimulator) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	// 1. Hikvision Mock ISAPI Server on 18081
	hikMux := http.NewServeMux()
	hikMux.HandleFunc("/ISAPI/System/deviceInfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <deviceName>HIKVISION ACUSENSE DOME</deviceName>
    <deviceID>88888888-8888-8888-8888-888888888888</deviceID>
    <model>DS-2CD2043G2-I</model>
    <serialNumber>DS-2CD2043G2-I20231015AAWR123456789</serialNumber>
    <macAddress>bc:54:51:2a:3c:91</macAddress>
    <firmwareVersion>V5.7.1 build 230915</firmwareVersion>
    <deviceType>IPCamera</deviceType>
</DeviceInfo>`)
	})

	hikMux.HandleFunc("/ISAPI/System/network/interfaces/1/ipAddress", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == "PUT" {
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "<addressingType>dynamic</addressingType>") {
				s.hikDHCP = true
			} else {
				s.hikDHCP = false
			}
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ResponseStatus version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <statusCode>1</statusCode>
    <statusString>OK</statusString>
</ResponseStatus>`)
			return
		}
		addrType := "static"
		if s.hikDHCP {
			addrType = "dynamic"
		}
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<IPAddress version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <addressingType>%s</addressingType>
    <ipAddress>%s</ipAddress>
    <subnetMask>255.255.255.0</subnetMask>
    <DefaultGateway><ipAddress>192.168.1.1</ipAddress></DefaultGateway>
</IPAddress>`, addrType, s.hikIP)
	})

	hikMux.HandleFunc("/ISAPI/Image/channels/1/color", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == "PUT" {
			fmt.Fprintf(w, `<ResponseStatus version="2.0"><statusCode>1</statusCode><statusString>OK</statusString></ResponseStatus>`)
			return
		}
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<Color version="2.0">
    <brightnessLevel>%d</brightnessLevel>
    <contrastLevel>%d</contrastLevel>
    <saturationLevel>50</saturationLevel>
    <sharpnessLevel>50</sharpnessLevel>
</Color>`, s.hikBright, s.hikContrast)
	})

	hikMux.HandleFunc("/ISAPI/PTZCtrl/channels/1/continuous", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<ResponseStatus version="2.0"><statusCode>1</statusCode><statusString>OK</statusString></ResponseStatus>`)
	})

	hikMux.HandleFunc("/ISAPI/Streaming/channels/101/picture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#111827"/><text x="320" y="180" fill="#ef4444" font-family="monospace" font-size="20" text-anchor="middle">LIVE HIKVISION FEED 1080P</text></svg>`)
	})

	s.hikServer = &http.Server{Addr: "127.0.0.1:18081", Handler: hikMux}
	go s.hikServer.ListenAndServe()

	// 2. Dahua Mock CGI Server on 18082
	dahuaMux := http.NewServeMux()
	dahuaMux.HandleFunc("/cgi-bin/magicBox.cgi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "deviceType=DHI-XVR5108HS-4KL-I3\nserialNumber=9A038E8PAG10082\nappVersion=4.001.0000001.0\n")
	})

	dahuaMux.HandleFunc("/cgi-bin/configManager.cgi", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		name := r.URL.Query().Get("name")
		if action == "setConfig" {
			fmt.Fprintf(w, "OK\n")
			return
		}
		if name == "Network" {
			dhcpStr := "false"
			if s.dahuaDHCP {
				dhcpStr = "true"
			}
			fmt.Fprintf(w, "table.Network.eth0.IPAddress=%s\ntable.Network.eth0.SubnetMask=255.255.255.0\ntable.Network.eth0.DefaultGateway=192.168.1.1\ntable.Network.eth0.DhcpEnable=%s\n", s.dahuaIP, dhcpStr)
			return
		}
		if name == "VideoColor" {
			fmt.Fprintf(w, "table.VideoColor[0][0].Brightness=%d\ntable.VideoColor[0][0].Contrast=50\ntable.VideoColor[0][0].Saturation=50\ntable.VideoColor[0][0].Sharpness=50\n", s.dahuaBright)
			return
		}
		fmt.Fprintf(w, "table.CoaxialSensor[0].Format=%s\n", s.dahuaFormat)
	})

	dahuaMux.HandleFunc("/cgi-bin/ptz.cgi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK\n")
	})

	dahuaMux.HandleFunc("/cgi-bin/snapshot.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#030712"/><text x="320" y="180" fill="#0ea5e9" font-family="monospace" font-size="20" text-anchor="middle">LIVE DAHUA HDCVI FEED</text></svg>`)
	})

	s.dahuaServer = &http.Server{Addr: "127.0.0.1:18082", Handler: dahuaMux}
	go s.dahuaServer.ListenAndServe()

	// 3. Generic ONVIF SOAP Server on 18083
	onvifMux := http.NewServeMux()
	onvifMux.HandleFunc("/onvif/device_service", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		if strings.Contains(bodyStr, "GetDeviceInformation") {
			fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
	<soap:Body>
		<tds:GetDeviceInformationResponse>
			<tds:Manufacturer>CP PLUS</tds:Manufacturer>
			<tds:Model>CP-UNC-TA41L3-V2</tds:Model>
			<tds:FirmwareVersion>2.800.0000000.12.R</tds:FirmwareVersion>
			<tds:SerialNumber>CPP20241107A01</tds:SerialNumber>
		</tds:GetDeviceInformationResponse>
	</soap:Body>
</soap:Envelope>`)
			return
		}
		if strings.Contains(bodyStr, "GetNetworkInterfaces") {
			fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
	<soap:Body>
		<tds:GetNetworkInterfacesResponse>
			<tds:NetworkInterfaces token="eth0">
				<tt:IPv4>
					<tt:Enabled>true</tt:Enabled>
					<tt:Config>
						<tt:Manual><tt:Address>127.0.0.1</tt:Address><tt:PrefixLength>24</tt:PrefixLength></tt:Manual>
						<tt:DHCP>false</tt:DHCP>
					</tt:Config>
				</tt:IPv4>
			</tds:NetworkInterfaces>
		</tds:GetNetworkInterfacesResponse>
	</soap:Body>
</soap:Envelope>`)
			return
		}
		fmt.Fprintf(w, `<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"><soap:Body><Response>OK</Response></soap:Body></soap:Envelope>`)
	})

	onvifMux.HandleFunc("/onvif/ptz_service", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		fmt.Fprintf(w, `<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"><soap:Body><ContinuousMoveResponse/></soap:Body></soap:Envelope>`)
	})

	s.onvifServer = &http.Server{Addr: "127.0.0.1:18083", Handler: onvifMux}
	go s.onvifServer.ListenAndServe()

	// 4. Mock RTSP Server on port 18554
	l, err := net.Listen("tcp", "127.0.0.1:18554")
	if err == nil {
		s.rtspListener = l
		go func() {
			for {
				conn, err := s.rtspListener.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					buf := make([]byte, 1024)
					n, err := c.Read(buf)
					if err == nil && n > 0 {
						resp := "RTSP/1.0 200 OK\r\nCSeq: 1\r\nPublic: OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN\r\nServer: CCTV Sentinel Universal RTSP Simulator\r\n\r\n"
						c.Write([]byte(resp))
					}
				}(conn)
			}
		}()
	}

	s.running = true
	return nil
}

func (s *MultiDeviceSimulator) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	if s.hikServer != nil {
		s.hikServer.Close()
	}
	if s.dahuaServer != nil {
		s.dahuaServer.Close()
	}
	if s.onvifServer != nil {
		s.onvifServer.Close()
	}
	if s.rtspListener != nil {
		s.rtspListener.Close()
	}
	s.running = false
}
