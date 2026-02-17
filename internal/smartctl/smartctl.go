package smartctl

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/danweinerdev/smart-monitor/internal/discovery"
)

// SmartctlOutput represents the top-level JSON output from smartctl --json.
type SmartctlOutput struct {
	Smartctl    SmartctlInfo         `json:"smartctl"`
	Device      SmartctlDevice       `json:"device"`
	ModelName   string               `json:"model_name"`
	SerialNum   string               `json:"serial_number"`
	FWVersion   string               `json:"firmware_version"`
	Temperature *SmartctlTemperature `json:"temperature"`
	PowerOnTime *SmartctlPowerOn     `json:"power_on_time"`
	ATASmart    *ATASmartAttributes  `json:"ata_smart_attributes"`
	NVMESmart   *NVMESmartHealth     `json:"nvme_smart_health_information_log"`
}

type SmartctlInfo struct {
	ExitStatus int `json:"exit_status"`
}

type SmartctlDevice struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
}

type SmartctlTemperature struct {
	Current int `json:"current"`
}

type SmartctlPowerOn struct {
	Hours int64 `json:"hours"`
}

type ATASmartAttributes struct {
	Table []ATASmartAttribute `json:"table"`
}

type ATASmartAttribute struct {
	ID   int                    `json:"id"`
	Name string                 `json:"name"`
	Raw  ATASmartAttributeValue `json:"raw"`
}

type ATASmartAttributeValue struct {
	Value int64  `json:"value"`
	Str   string `json:"string"`
}

type NVMESmartHealth struct {
	Temperature      int   `json:"temperature"`
	AvailableSpare   int   `json:"available_spare"`
	PercentageUsed   int   `json:"percentage_used"`
	DataUnitsRead    int64 `json:"data_units_read"`
	DataUnitsWritten int64 `json:"data_units_written"`
	PowerOnHours     int64 `json:"power_on_hours"`
	UnsafeShutdowns  int64 `json:"unsafe_shutdowns"`
	MediaErrors      int64 `json:"media_errors"`
}

// DriveMetrics holds the parsed SMART data for one drive.
type DriveMetrics struct {
	Drive  discovery.Drive
	Model  string
	Serial string
	Fields map[string]interface{}
}

// SmartctlRunner abstracts smartctl execution.
type SmartctlRunner interface {
	Run(drive discovery.Drive) (*SmartctlOutput, error)
}

// ExecSmartctl runs the real smartctl binary.
type ExecSmartctl struct {
	Path string
}

func NewExecSmartctl() *ExecSmartctl {
	return &ExecSmartctl{Path: "smartctl"}
}

func (e *ExecSmartctl) Run(drive discovery.Drive) (*SmartctlOutput, error) {
	args := []string{"--json", "-a"}
	if drive.Type == discovery.DriveTypeSATA {
		args = append(args, "-d", "sat")
	}
	args = append(args, drive.Path)

	cmd := exec.Command(e.Path, args...)
	out, err := cmd.Output()

	// smartctl uses exit bit flags — non-zero exit doesn't always mean failure.
	// Only fail if we got no output at all.
	if len(out) == 0 {
		if err != nil {
			return nil, fmt.Errorf("smartctl returned no output: %w", err)
		}
		return nil, fmt.Errorf("smartctl returned no output")
	}

	var result SmartctlOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing smartctl JSON: %w", err)
	}

	return &result, nil
}

// SATA attribute IDs we care about.
var sataAttributeIDs = map[int]string{
	1:   "raw_read_error_rate",
	5:   "reallocated_sector_count",
	7:   "seek_error_rate",
	9:   "power_on_hours",
	10:  "spin_retry_count",
	194: "temperature",
	197: "current_pending_sector",
	198: "offline_uncorrectable",
}

// ParseMetrics extracts DriveMetrics from smartctl output.
func ParseMetrics(drive discovery.Drive, output *SmartctlOutput) (*DriveMetrics, error) {
	metrics := &DriveMetrics{
		Drive:  drive,
		Model:  output.ModelName,
		Serial: output.SerialNum,
		Fields: make(map[string]interface{}),
	}

	switch drive.Type {
	case discovery.DriveTypeSATA:
		if output.ATASmart == nil {
			return nil, fmt.Errorf("no ata_smart_attributes in output for %s", drive.Path)
		}
		for _, attr := range output.ATASmart.Table {
			if fieldName, ok := sataAttributeIDs[attr.ID]; ok {
				metrics.Fields[fieldName] = attr.Raw.Value
			}
		}
		// Temperature fallback: if attribute 194 wasn't found, use top-level temperature.
		if _, ok := metrics.Fields["temperature"]; !ok {
			if output.Temperature != nil {
				metrics.Fields["temperature"] = int64(output.Temperature.Current)
			}
		}

	case discovery.DriveTypeNVMe:
		if output.NVMESmart == nil {
			return nil, fmt.Errorf("no nvme_smart_health_information_log in output for %s", drive.Path)
		}
		nvme := output.NVMESmart
		metrics.Fields["temperature"] = int64(nvme.Temperature)
		metrics.Fields["power_on_hours"] = nvme.PowerOnHours
		metrics.Fields["percentage_used"] = int64(nvme.PercentageUsed)
		metrics.Fields["available_spare"] = int64(nvme.AvailableSpare)
		metrics.Fields["media_errors"] = nvme.MediaErrors
		metrics.Fields["unsafe_shutdowns"] = nvme.UnsafeShutdowns
		metrics.Fields["data_units_read"] = nvme.DataUnitsRead
		metrics.Fields["data_units_written"] = nvme.DataUnitsWritten

	default:
		return nil, fmt.Errorf("unknown drive type: %s", drive.Type)
	}

	return metrics, nil
}
