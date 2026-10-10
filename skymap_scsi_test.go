package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tidwall/gjson"
)

// Feed deterministic synthetic smartctl JSON to the actual collector.
// No devices, privileged Docker containers, or host smartctl binaries are needed.
type skymapSCSIFixtureCollector struct {
	device Device
}

func (c *skymapSCSIFixtureCollector) Describe(ch chan<- *prometheus.Desc) {
	manager := &SMARTctlManagerCollector{}
	manager.Describe(ch)
}

func (c *skymapSCSIFixtureCollector) Collect(ch chan<- prometheus.Metric) {
	// The returned device.type determines whether the SCSI collectors run.
	data := gjson.Parse(
		`{"smartctl":{"exit_status":0}, "device":{"type":"` +
			c.device.Type +
			`","protocol":"SCSI"}, "scsi_model_name":"TEST SAS DISK",
            "scsi_grown_defect_list":20,
            "scsi_percentage_used_endurance_indicator":7,
            "scsi_error_counter_log":{
                "read":{"total_uncorrected_errors":0,"gigabytes_processed":42},
                "write":{"total_uncorrected_errors":0,"gigabytes_processed":25},
                "verify":{"total_uncorrected_errors":0}
            }}`,
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	smart := NewSMARTctl(logger, data, ch, c.device)
	smart.Collect()
}

func skymapGatherSCSIMetricNames(t *testing.T, deviceType string) map[string]bool {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	collector := &skymapSCSIFixtureCollector{
		device: Device{
			Name:  "/dev/bus/0",
			Type:  deviceType,
			Label: "test_disk",
		},
	}
	if err := registry.Register(collector); err != nil {
		t.Fatalf("register fixture collector: %v", err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("SCSI metrics were emitted without registered descriptors: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	return names
}

func TestSCSIMetricDescriptorsRegistered(t *testing.T) {
	names := skymapGatherSCSIMetricNames(t, "scsi")
	for _, metric := range []string{
		"smartctl_scsi_percentage_used_endurance_indicator",
		"smartctl_verify_errors_corrected_by_rereads_rewrites",
		"smartctl_verify_errors_corrected_by_eccfast",
		"smartctl_verify_errors_corrected_by_eccdelayed",
		"smartctl_verify_total_uncorrected_errors",
	} {
		if !names[metric] {
			t.Errorf("missing SCSI metric %s", metric)
		}
	}
}

func TestMegaRAIDCollectsSCSIMetrics(t *testing.T) {
	names := skymapGatherSCSIMetricNames(t, "megaraid,0")
	for _, metric := range []string{
		"smartctl_scsi_grown_defect_list",
		"smartctl_read_total_uncorrected_errors",
		"smartctl_write_total_uncorrected_errors",
		"smartctl_device_bytes_read",
		"smartctl_device_bytes_written",
	} {
		if !names[metric] {
			t.Errorf("MegaRAID SCSI device did not expose %s", metric)
		}
	}
}
