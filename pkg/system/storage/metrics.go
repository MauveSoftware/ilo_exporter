// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import (
	"strings"
	"sync"

	"github.com/MauveSoftware/ilo4_exporter/pkg/client"
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prefix = "ilo4_storage_"
)

var (
	logicalDriveCapacityDesc *prometheus.Desc
	logicalDriveHealthyDesc  *prometheus.Desc
	diskDriveTempDesc        *prometheus.Desc
	diskDriveCapacityDesc    *prometheus.Desc
	diskDriveRotationDesc    *prometheus.Desc
	diskDriveHealthyDesc     *prometheus.Desc
)

func init() {
	ll := []string{"host", "array_controller", "name", "raid"}
	logicalDriveCapacityDesc = prometheus.NewDesc(prefix+"logical_capacity_byte", "Capacity of the logical drive in bytes", ll, nil)
	logicalDriveHealthyDesc = prometheus.NewDesc(prefix+"logical_healthy", "Health status of the logical drive", ll, nil)

	dl := []string{"host", "array_controller", "location", "model", "interface_type"}
	diskDriveCapacityDesc = prometheus.NewDesc(prefix+"disk_capacity_byte", "Capacity of the disk in bytes", dl, nil)
	diskDriveHealthyDesc = prometheus.NewDesc(prefix+"disk_healthy", "Health status of the diks", dl, nil)
	diskDriveRotationDesc = prometheus.NewDesc(prefix+"disk_rpm", "Disk rotations / minute", dl, nil)
	diskDriveTempDesc = prometheus.NewDesc(prefix+"disk_temperature", "Temperature of the disc in degree celsius", dl, nil)
}

// Describe describes all metrics for the storage package
func Describe(ch chan<- *prometheus.Desc) {
	ch <- logicalDriveCapacityDesc
	ch <- logicalDriveHealthyDesc
	ch <- diskDriveTempDesc
	ch <- diskDriveCapacityDesc
	ch <- diskDriveRotationDesc
	ch <- diskDriveHealthyDesc
}

// Collect collects metrics for storage controllers
func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	p := parentPath + "/SmartStorage/ArrayControllers"
	crtls := common.ResourceLinks{}

	err := cl.Get(p, &crtls)
	if err != nil {
		errCh <- errors.Wrap(err, "could not get array controller summary")
		return
	}

	wg.Add(len(crtls.Links.Members))

	for _, l := range crtls.Links.Members {
		go collectForArrayController(l.Href, cl, ch, wg, errCh)
	}
}

func collectForArrayController(link string, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	i := strings.Index(link, "Systems/")
	p := link[i:]

	crtl := ArrayController{}

	err := cl.Get(p, &crtl)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get array controller information from %s", link)
		return
	}

	wg.Add(2)

	go collectLogicalDrives(p, crtl, cl, ch, wg, errCh)
	go collectDiskDrives(p, crtl, cl, ch, wg, errCh)
}

func collectLogicalDrives(parentPath string, crtl ArrayController, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	drvs, err := driveLinks(parentPath+"/"+"LogicalDrives", cl)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get logical drive information for array controller %s", crtl.SerialNumber)
		return
	}

	wg.Add(len(drvs))
	for _, d := range drvs {
		go collectLogicalDrive(d, crtl, cl, ch, wg, errCh)
	}
}

func collectLogicalDrive(path string, crtl ArrayController, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	d := LogicalDrive{}

	err := cl.Get(path, &d)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get drive information from %s", path)
		return
	}

	l := []string{cl.HostName(), crtl.SerialNumber, d.LogicalDriveName, d.Raid}
	ch <- prometheus.MustNewConstMetric(logicalDriveCapacityDesc, prometheus.GaugeValue, float64(d.CapacityMiB<<20), l...)
	ch <- prometheus.MustNewConstMetric(logicalDriveHealthyDesc, prometheus.GaugeValue, d.Status.HealthValue(), l...)
}

func collectDiskDrives(parentPath string, crtl ArrayController, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	drvs, err := driveLinks(parentPath+"/"+"DiskDrives", cl)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get disk drive information for array controller %s", crtl.SerialNumber)
		return
	}

	wg.Add(len(drvs))
	for _, d := range drvs {
		go collectDiskDrive(d, crtl, cl, ch, wg, errCh)
	}
}

func collectDiskDrive(path string, crtl ArrayController, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	d := DiskDrive{}

	err := cl.Get(path, &d)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get drive information from %s", path)
		return
	}

	l := []string{cl.HostName(), crtl.SerialNumber, d.Location, d.Model, d.InterfaceType}
	ch <- prometheus.MustNewConstMetric(diskDriveCapacityDesc, prometheus.GaugeValue, float64(d.CapacityGB<<30), l...)
	ch <- prometheus.MustNewConstMetric(diskDriveHealthyDesc, prometheus.GaugeValue, d.Status.HealthValue(), l...)
	ch <- prometheus.MustNewConstMetric(diskDriveRotationDesc, prometheus.GaugeValue, d.RotationalSpeedRpm, l...)
	ch <- prometheus.MustNewConstMetric(diskDriveTempDesc, prometheus.GaugeValue, d.CurrentTemperatureCelsius, l...)
}

func driveLinks(path string, cl client.Client) ([]string, error) {
	drvLinks := common.ResourceLinks{}

	err := cl.Get(path, &drvLinks)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get drive list from %s", path)
	}

	drvs := make([]string, len(drvLinks.Links.Members))
	for i, l := range drvLinks.Links.Members {
		drvs[i] = l.Href[strings.Index(l.Href, "Systems/"):]
	}

	return drvs, nil
}
