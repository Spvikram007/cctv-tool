package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/cpplus"
	"cctv-sentinel/pkg/adapters/dahua"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/adapters/onvif"
	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func main() {
	log.Println("=================================================================")
	log.Println(" CCTV SENTINEL UNIVERSAL — Enterprise CCTV Technician Platform")
	log.Println(" Owner: Tech Defence Solutions")
	log.Println(" Tagline: ONE TOOL. EVERY CCTV. COMPLETE CONTROL.")
	log.Println("=================================================================")

	// 1. Initialize simulator for verified hardware emulation
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		log.Printf("[SIMULATOR] Notice: %v", err)
	} else {
		log.Println("[SIMULATOR] Hardware simulator active:")
		log.Println("  -> Hikvision ISAPI: http://127.0.0.1:18081")
		log.Println("  -> Dahua CGI:      http://127.0.0.1:18082")
		log.Println("  -> Generic ONVIF:  http://127.0.0.1:18083")
		log.Println("  -> Mock RTSP:      rtsp://127.0.0.1:18554")
	}
	time.Sleep(100 * time.Millisecond)

	// 2. Core database & audit logging
	dbFile := "sentinel_store.json"
	store := database.NewStore(dbFile)
	logger := audit.NewLogger()
	licenseMgr := licensing.NewManager()

	// 3. Adapter Registry
	registry := adapters.NewRegistry()
	onvifAdapter := onvif.NewONVIFAdapter()
	hikAdapter := hikvision.NewHikvisionAdapter()
	dahuaAdapter := dahua.NewDahuaAdapter()
	cpAdapter := cpplus.NewCPPlusAdapter()

	registry.Register(onvifAdapter, models.ProtocolONVIF)
	registry.Register(hikAdapter, models.ProtocolISAPI)
	registry.Register(dahuaAdapter, models.ProtocolDahuaCGI)
	registry.Register(cpAdapter, models.ProtocolType("CP_PLUS_SDK"))

	// 4. Engines
	configEng := configengine.NewEngine(registry, store, logger)
	discoveryEng := discovery.NewEngine(registry)

	// Seed genuine simulated test devices
	if len(store.ListDevices()) == 0 {
		store.UpsertDevice(models.Device{
			ID:           "hik-127-0-0-1-18081",
			Name:         "Hikvision AcuSense DS-2CD2043G2-I",
			IP:           "127.0.0.1",
			Port:         18081,
			RTSPPort:     18554,
			ONVIFPort:    18081,
			Manufacturer: models.VendorHikvision,
			Model:        "DS-2CD2043G2-I",
			Firmware:     "V5.7.1 build 230915",
			SerialNumber: "DS-2CD2043G2-I20231015AAWR123456789",
			Category:     models.CategoryIPCamera,
			Status:       "online",
			Protocols:    []models.ProtocolType{models.ProtocolISAPI, models.ProtocolONVIF, models.ProtocolRTSP},
			Capabilities: []string{"ISAPI_CONFIG", "STREAMING", "IMAGE_SETTINGS", "ENCODING", "ANALOG_UTC", "PTZ", "SNAPSHOT"},
			ChannelCount: 1,
			Username:     "admin",
			Password:     "Admin@123",
			LastSeen:     time.Now().UTC(),
		})

		store.UpsertDevice(models.Device{
			ID:           "dahua-127-0-0-1-18082",
			Name:         "Dahua WizSense XVR5108HS-4KL-I3",
			IP:           "127.0.0.1",
			Port:         18082,
			RTSPPort:     18554,
			ONVIFPort:    18082,
			Manufacturer: models.VendorDahua,
			Model:        "DHI-XVR5108HS-4KL-I3",
			Firmware:     "4.001.0000001.0",
			SerialNumber: "9A038E8PAG10082",
			Category:     models.CategoryXVR,
			Status:       "online",
			Protocols:    []models.ProtocolType{models.ProtocolDahuaCGI, models.ProtocolONVIF, models.ProtocolRTSP},
			Capabilities: []string{"CGI_CONFIG", "STREAMING", "IMAGE_SETTINGS", "ENCODING", "HDCVI_UTC", "PTZ", "SNAPSHOT"},
			ChannelCount: 8,
			Username:     "admin",
			Password:     "Admin@123",
			LastSeen:     time.Now().UTC(),
		})

		store.UpsertDevice(models.Device{
			ID:           "cpplus-127-0-0-1-18083",
			Name:         "CP PLUS Indigo CP-UNC-TA41L3-V2",
			IP:           "127.0.0.1",
			Port:         18083,
			RTSPPort:     18554,
			ONVIFPort:    18083,
			Manufacturer: models.VendorCPPlus,
			Model:        "CP-UNC-TA41L3-V2",
			Firmware:     "2.800.0000000.12.R",
			SerialNumber: "CPP20241107A01",
			Category:     models.CategoryIPCamera,
			Status:       "online",
			Protocols:    []models.ProtocolType{models.ProtocolONVIF, models.ProtocolRTSP},
			Capabilities: []string{"STREAMING", "IMAGE_SETTINGS", "ENCODING", "SNAPSHOT", "PTZ"},
			ChannelCount: 1,
			Username:     "admin",
			Password:     "Admin@123",
			LastSeen:     time.Now().UTC(),
		})

		logger.Log("System", "SYSTEM", "127.0.0.1", "INITIALIZE", "SYSTEM", "SUCCESS", "Seeded verified hardware simulation devices", "")
		log.Println("[SETUP] Seeded 3 verified hardware units into fleet inventory.")
	}

	// 5. REST API Server
	apiServer := api.NewServer(store, registry, configEng, discoveryEng, licenseMgr, logger)

	serverPort := 8080
	srv := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", serverPort),
		Handler:      apiServer.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n[SYSTEM] Shutting down CCTV Sentinel Universal gracefully...")
		sim.Stop()
		_ = store.Save()
		os.Exit(0)
	}()

	log.Printf("[REST API] CCTV Sentinel Universal API listening on http://127.0.0.1:%d", serverPort)
	log.Printf("[REST API] OpenAPI endpoints ready at /api/v1/system/status, /api/v1/devices, /api/v1/diagnostics")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}
