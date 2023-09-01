// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"fmt"

	"github.com/MauveSoftware/ilo5_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo_storage_"
)

var (
	diskDriveCapacityDesc *prometheus.Desc
	diskDriveHealthyDesc  *prometheus.Desc
)

func init() {
	dl := []string{"host", "location", "model", "media_type"}
	diskDriveCapacityDesc = prometheus.NewDesc(prefix+"disk_capacity_byte", "Capacity of the disk in bytes", dl, nil)
	diskDriveHealthyDesc = prometheus.NewDesc(prefix+"disk_healthy", "Health status of the diks", dl, nil)
}

// Describe describes all metrics for the storage package
func Describe(ch chan<- *prometheus.Desc) {
	ch <- diskDriveCapacityDesc
	ch <- diskDriveHealthyDesc
}

// Collect collects metrics for storage controllers
func Collect(parentPath string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(cc.RootCtx(), "Storage.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	collectStorage(ctx, parentPath, cc)
	collectSmartStorage(ctx, parentPath, cc)
}

func collectStorage(ctx context.Context, parentPath string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectStorage", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	p := parentPath + "/Storage"
	crtls := common.MemberList{}

	err := cc.Client().Get(ctx, p, &crtls)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get storage controller summary: %w", err), span)
		return
	}

	for _, l := range crtls.Members {
		collectStorageController(ctx, l.Path, cc)
	}
}

func collectStorageController(ctx context.Context, path string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectController", trace.WithAttributes(
		attribute.String("path", path),
	))
	defer span.End()

	strg := StorageInfo{}
	err := cc.Client().Get(ctx, path, &strg)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get storage controller summary: %w", err), span)
		return
	}

	for _, drv := range strg.Drives {
		collectDiskDrive(ctx, drv.Path, cc)
	}
}

func collectSmartStorage(ctx context.Context, parentPath string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectSmartStorage", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	p := parentPath + "/SmartStorage/ArrayControllers/"
	crtls := common.MemberList{}
	err := cc.Client().Get(ctx, p, &crtls)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get smart storage controller summary: %w", err), span)
		return
	}

	for _, m := range crtls.Members {
		collectSmartStorageController(ctx, m.Path, cc)
	}
}

func collectSmartStorageController(ctx context.Context, path string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectSmartController", trace.WithAttributes(
		attribute.String("path", path),
	))
	defer span.End()

	p := path + "DiskDrives/"
	drives := common.MemberList{}
	err := cc.Client().Get(ctx, p, &drives)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get drives for controller %s: %w", path, err), span)
		return
	}

	for _, drv := range drives.Members {
		collectDiskDrive(ctx, drv.Path, cc)
	}
}

func collectDiskDrive(ctx context.Context, path string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectDisk", trace.WithAttributes(
		attribute.String("path", path),
	))
	defer span.End()

	d := DiskDrive{}
	err := cc.Client().Get(ctx, path, &d)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get drive information from %s: %w", path, err), span)
		return
	}

	l := []string{cc.Client().HostName(), string(d.Location), d.Model, d.MediaType}
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(diskDriveCapacityDesc, prometheus.GaugeValue, float64(d.CapacityBytes()), l...),
		prometheus.MustNewConstMetric(diskDriveHealthyDesc, prometheus.GaugeValue, d.Status.HealthValue(), l...),
	)
}
