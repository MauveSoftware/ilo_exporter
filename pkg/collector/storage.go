// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	Drives []redfish.Link `json:"Drives"`
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

// GetLocation returns the location of the drive
func (d *DiskDrive) GetLocation() string {
	if d.Location != "" {
		return string(d.Location)
	}

	return string(d.PhysicalLocation)
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

func collectStorage(ctx context.Context, s *scrape) error {
	errs := make(chan error, 2)
	go func() { errs <- collectStorageControllers(ctx, s) }()
	go func() { errs <- collectSmartStorage(ctx, s) }()

	return errors.Join(<-errs, <-errs)
}

func collectStorageControllers(ctx context.Context, s *scrape) error {
	ctrls := redfish.Collection{}
	if err := s.cl.Get(ctx, s.systemPath+"/Storage", &ctrls); err != nil {
		return fmt.Errorf("could not get storage controller summary: %w", err)
	}

	return forEach(ctx, ctrls.Members, func(ctx context.Context, l redfish.Link) error {
		strg := Storage{}
		if err := s.cl.Get(ctx, l.ID, &strg); err != nil {
			return fmt.Errorf("could not get storage controller summary: %w", err)
		}

		return forEach(ctx, strg.Drives, func(ctx context.Context, l redfish.Link) error {
			return collectDiskDrive(ctx, s, l.ID)
		})
	})
}

func collectSmartStorage(ctx context.Context, s *scrape) error {
	ctrls := redfish.Collection{}
	if err := s.cl.Get(ctx, s.systemPath+"/SmartStorage/ArrayControllers/", &ctrls); err != nil {
		return fmt.Errorf("could not get smart storage controller summary: %w", err)
	}

	return forEach(ctx, ctrls.Members, func(ctx context.Context, l redfish.Link) error {
		drives := redfish.Collection{}
		if err := s.cl.Get(ctx, l.ID+"DiskDrives/", &drives); err != nil {
			return fmt.Errorf("could not get drives for controller %s: %w", l.ID, err)
		}

		return forEach(ctx, drives.Members, func(ctx context.Context, l redfish.Link) error {
			return collectDiskDrive(ctx, s, l.ID)
		})
	})
}

func collectDiskDrive(ctx context.Context, s *scrape, path string) error {
	d := DiskDrive{}
	if err := s.cl.Get(ctx, path, &d); err != nil {
		return fmt.Errorf("could not get drive information from %s: %w", path, err)
	}

	labels := []string{s.host, d.GetLocation(), d.Model, d.MediaType, d.Name, d.SerialNumber}
	s.emit(diskCapacityDesc, d.CapacityBytes(), labels...)
	s.emit(diskHealthyDesc, d.Status.HealthValue(), labels...)
	s.emit(diskFailurePredictedDesc, d.FailurePredictedValue(), labels...)
	return nil
}
