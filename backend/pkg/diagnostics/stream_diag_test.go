package diagnostics

import (
	"fmt"
	"net"
	"testing"

	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func TestRunStreamDiagnosticsSimulator(t *testing.T) {
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("failed to start simulator: %v", err)
	}
	defer sim.Stop()

	dev := &models.Device{
		ID:       "sim-dev",
		IP:       "127.0.0.1",
		RTSPPort: 18554,
	}

	report := RunStreamDiagnostics(dev, "rtsp://127.0.0.1:18554/live", "admin", "Admin123")

	if !report.RTSPConnected {
		t.Errorf("expected RTSPConnected=true on 18554")
	}
	if len(report.OptionsMethods) == 0 {
		t.Errorf("expected options methods returned from simulator")
	}
}

func TestRunStreamDiagnosticsUnreachable(t *testing.T) {
	// Pick unused port
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	dev := &models.Device{
		ID:       "dead-dev",
		IP:       "127.0.0.1",
		RTSPPort: port,
	}

	report := RunStreamDiagnostics(dev, fmt.Sprintf("rtsp://127.0.0.1:%d/live", port), "", "")
	if report.RTSPConnected {
		t.Errorf("expected RTSPConnected=false for closed port")
	}
	if len(report.Recommendations) == 0 {
		t.Errorf("expected recommendations for unreachable target")
	}
}
