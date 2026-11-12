// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
)

const storagePrefix = prefix + "storage_"

var (
	diskLabels               = []string{"host", "location", "model", "media_type", "name", "serial_number"}
	diskCapacityDesc         = newDesc(storagePrefix+"disk_capacity_byte", "Capacity of the disk in bytes", diskLabels...)
	diskHealthyDesc          = newDesc(storagePrefix+"disk_healthy", "Health status of the disk", diskLabels...)
	diskFailurePredictedDesc = newDesc(storagePrefix+"disk_failure_predicted", "Failure of disk is predicted", diskLabels...)
)

// Storage is a storage subsystem resource
type Storage struct {
	// Drives are references to drives or drives (if expanded)
	Drives []json.RawMessage `json:"Drives"`
}

// DiskDrive is a drive resource (DMTF Drive or HPE SmartStorage DiskDrive)
type DiskDrive struct {
	Name             string         `json:"Name"`
	SerialNumber     string         `json:"SerialNumber"`
	MediaType        string         `json:"MediaType"`
	Model            string         `json:"Model"`
	Location         Location       `json:"Location"`
	PhysicalLocation Location       `json:"PhysicalLocation"`
	CapacityB        uint64         `json:"CapacityBytes"`
	CapacityMB       uint64         `json:"CapacityMiB"`
	Status           redfish.Status `json:"Status"`
	FailurePredicted bool           `json:"FailurePredicted"`
}

// GetLocation returns the location of the drive. The physical location (e.g. "Embedded:Port=5A:Box=1:Bay=3")
// is preferred over the deprecated location, which only contains box and bay (e.g. "1:3") for drives attached
// to the system. SmartStorage drives only have a location (e.g. "2I:1:1").
func (d *DiskDrive) GetLocation() string {
	if d.PhysicalLocation != "" {
		return string(d.PhysicalLocation)
	}

	return string(d.Location)
}

// CapacityBytes returns the capacity in bytes
func (d *DiskDrive) CapacityBytes() float64 {
	if d.CapacityMB > 0 {
		return float64(d.CapacityMB << 20)
	}

	return float64(d.CapacityB)
}

// FailurePredictedValue returns 1 if a failure is predicted, 0 otherwise
func (d *DiskDrive) FailurePredictedValue() float64 {
	if d.FailurePredicted {
		return 1
	}

	return 0
}

// Location is the location of a drive. It can be given as string (SmartStorage), as array of
// location infos (DMTF Location, deprecated) or as part location (DMTF PhysicalLocation).
type Location string

type locationInfo struct {
	Info string `json:"Info"`
}

type physicalLocationInfo struct {
	PartLocation struct {
		ServiceLabel string `json:"ServiceLabel"` // e.g. "Slot=12:Port=1:Box=1:Bay=1"
	} `json:"PartLocation"`
}

// UnmarshalJSON implements json.Unmarshaler
func (f *Location) UnmarshalJSON(data []byte) error {
	var infos []locationInfo
	_ = json.Unmarshal(data, &infos)

	if len(infos) > 0 {
		*f = Location(infos[0].Info)
		return nil
	}

	var info physicalLocationInfo
	_ = json.Unmarshal(data, &info)

	if info.PartLocation.ServiceLabel != "" {
		*f = Location(info.PartLocation.ServiceLabel)
		return nil
	}

	var str string
	_ = json.Unmarshal(data, &str)
	*f = Location(str)
	return nil
}

// SmartArrayController is an array controller of the HPE SmartStorage API
type SmartArrayController struct {
	Links struct {
		PhysicalDrives redfish.Link `json:"PhysicalDrives"`
	} `json:"Links"`
}

// collectStorage collects drives of the DMTF storage model and of the HPE SmartStorage API (iLO 4 and 5).
// iLO 5 reports drives of Smart Array controllers supporting RDE in both, so drives are deduplicated by
// serial number, preferring the DMTF model.
func collectStorage(ctx context.Context, s *scrape) error {
	var dmtf, smart []DiskDrive
	var dmtfErr, smartErr error

	var wg sync.WaitGroup
	wg.Go(func() { dmtf, dmtfErr = collectStorageControllers(ctx, s) })
	wg.Go(func() { smart, smartErr = collectSmartStorage(ctx, s) })
	wg.Wait()

	serials := make(map[string]bool)
	for _, d := range dmtf {
		if d.SerialNumber != "" {
			serials[d.SerialNumber] = true
		}
		emitDiskDrive(s, &d)
	}

	for _, d := range smart {
		if d.SerialNumber != "" && serials[d.SerialNumber] {
			continue
		}
		emitDiskDrive(s, &d)
	}

	return errors.Join(dmtfErr, smartErr)
}

func collectStorageControllers(ctx context.Context, s *scrape) ([]DiskDrive, error) {
	path := s.systemLink(func(sys *System) string { return sys.Storage.ID }, "Storage")
	if path == "" {
		return nil, nil
	}

	ctrls := redfish.Collection{}
	if err := s.cl.Get(ctx, path, &ctrls); err != nil {
		return nil, fmt.Errorf("could not get storage controller summary: %w", err)
	}

	// typically there are less controllers than drives, so the drives are expanded with each controller
	drives, err := collectAll(ctx, ctrls.Members, func(ctx context.Context, l redfish.Link) ([]DiskDrive, error) {
		strg := Storage{}
		if err := s.getExpanded(ctx, l.ID, &strg); err != nil {
			return nil, fmt.Errorf("could not get storage controller %s: %w", l.ID, err)
		}

		return resolve[DiskDrive](ctx, s, strg.Drives)
	})

	return slices.Concat(drives...), err
}

func collectSmartStorage(ctx context.Context, s *scrape) ([]DiskDrive, error) {
	path := s.systemLink(func(sys *System) string {
		if l := sys.SmartStorageLink(); l != "" {
			return strings.TrimSuffix(l, "/") + "/ArrayControllers/"
		}
		return ""
	}, "SmartStorage/ArrayControllers/")
	if path == "" {
		return nil, nil
	}

	ctrls, err := getMembers[SmartArrayController](ctx, s, path)
	if err != nil && len(ctrls) == 0 {
		return nil, fmt.Errorf("could not get smart storage controllers: %w", err)
	}

	drives, drvErr := collectAll(ctx, ctrls, func(ctx context.Context, ctrl SmartArrayController) ([]DiskDrive, error) {
		if ctrl.Links.PhysicalDrives.ID == "" {
			return nil, nil
		}

		return getMembers[DiskDrive](ctx, s, ctrl.Links.PhysicalDrives.ID)
	})

	return slices.Concat(drives...), errors.Join(err, drvErr)
}

func emitDiskDrive(s *scrape, d *DiskDrive) {
	if d.Status.IsAbsent() {
		// empty bay
		return
	}

	labels := []string{s.host, d.GetLocation(), d.Model, d.MediaType, d.Name, d.SerialNumber}
	s.emit(diskCapacityDesc, d.CapacityBytes(), labels...)
	s.emit(diskHealthyDesc, d.Status.HealthValue(), labels...)
	s.emit(diskFailurePredictedDesc, d.FailurePredictedValue(), labels...)
}
