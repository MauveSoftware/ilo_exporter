// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

// Package topology discovers the capabilities and resource layout of a Redfish service.
// Discovered topologies are cached, so discovery costs requests only once per host and TTL.
package topology

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/lru"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
	"golang.org/x/sync/singleflight"
)

// discoveryTimeout limits a discovery, which is shared by all scrapes of a host
const discoveryTimeout = 15 * time.Second

// Topology describes the resources of a host which rarely change
type Topology struct {
	// RedfishVersion is the version of the Redfish service
	RedfishVersion string

	// Expand is true if the service supports $expand=. (expansion of all subordinate resources but links)
	Expand bool

	// SystemPath is the path of the computer system
	SystemPath string

	// ChassisPath is the path of the chassis containing the system
	ChassisPath string

	// Chassis contains the links of the chassis resource
	Chassis ChassisLinks
}

// ChassisLinks are links of a chassis resource. Empty links are not supported by the service.
type ChassisLinks struct {
	Power              string
	Thermal            string
	PowerSubsystem     string
	ThermalSubsystem   string
	Sensors            string
	EnvironmentMetrics string
}

type serviceRoot struct {
	RedfishVersion            string       `json:"RedfishVersion"`
	Systems                   redfish.Link `json:"Systems"`
	ProtocolFeaturesSupported struct {
		ExpandQuery struct {
			ExpandAll bool `json:"ExpandAll"`
			NoLinks   bool `json:"NoLinks"`
		} `json:"ExpandQuery"`
	} `json:"ProtocolFeaturesSupported"`
}

type system struct {
	Links struct {
		Chassis []redfish.Link `json:"Chassis"`
	} `json:"Links"`
}

type chassis struct {
	Power              redfish.Link `json:"Power"`
	Thermal            redfish.Link `json:"Thermal"`
	PowerSubsystem     redfish.Link `json:"PowerSubsystem"`
	ThermalSubsystem   redfish.Link `json:"ThermalSubsystem"`
	Sensors            redfish.Link `json:"Sensors"`
	EnvironmentMetrics redfish.Link `json:"EnvironmentMetrics"`
}

// Discover discovers the topology by following links from the service root
func Discover(ctx context.Context, cl client.Client) (*Topology, error) {
	root := serviceRoot{}
	if err := cl.Get(ctx, "/redfish/v1/", &root); err != nil {
		return nil, fmt.Errorf("could not get service root: %w", err)
	}

	t := &Topology{
		RedfishVersion: root.RedfishVersion,
		Expand:         root.ProtocolFeaturesSupported.ExpandQuery.NoLinks || root.ProtocolFeaturesSupported.ExpandQuery.ExpandAll,
	}

	systems := redfish.Collection{}
	if err := cl.Get(ctx, root.Systems.ID, &systems); err != nil {
		return nil, fmt.Errorf("could not get systems: %w", err)
	}
	if len(systems.Members) == 0 {
		return nil, errors.New("no system found")
	}
	t.SystemPath = systems.Members[0].ID

	sys := system{}
	if err := cl.Get(ctx, t.SystemPath, &sys); err != nil {
		return nil, fmt.Errorf("could not get system: %w", err)
	}

	// the chassis collection also contains drive enclosures etc., so the chassis is taken from the system
	if len(sys.Links.Chassis) == 0 {
		return t, nil
	}
	t.ChassisPath = sys.Links.Chassis[0].ID

	ch := chassis{}
	if err := cl.Get(ctx, t.ChassisPath, &ch); err != nil {
		return nil, fmt.Errorf("could not get chassis: %w", err)
	}

	t.Chassis = ChassisLinks{
		Power:              ch.Power.ID,
		Thermal:            ch.Thermal.ID,
		PowerSubsystem:     ch.PowerSubsystem.ID,
		ThermalSubsystem:   ch.ThermalSubsystem.ID,
		Sensors:            ch.Sensors.ID,
		EnvironmentMetrics: ch.EnvironmentMetrics.ID,
	}

	return t, nil
}

// Cache caches topologies per host
type Cache struct {
	entries *lru.Cache[*Topology]
	group   singleflight.Group
}

// NewCache creates a cache for at most maxHosts hosts. Entries expire after ttl.
func NewCache(maxHosts int, ttl time.Duration) *Cache {
	return &Cache{
		entries: lru.New(maxHosts, lru.WithTTL[*Topology](ttl)),
	}
}

// Get returns the cached topology of host or discovers it. Concurrent calls for the same host share
// a single discovery, which is not canceled when ctx is done, so it can complete for later scrapes.
func (c *Cache) Get(ctx context.Context, host string, discover func(context.Context) (*Topology, error)) (*Topology, error) {
	key := Key(host)
	if t, found := c.entries.Get(key); found {
		return t, nil
	}

	ch := c.group.DoChan(key, func() (any, error) {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), discoveryTimeout)
		defer cancel()

		t, err := discover(dctx)
		if err != nil {
			return nil, err
		}

		c.entries.Set(key, t)
		return t, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*Topology), nil
	case <-ctx.Done():
		return nil, fmt.Errorf("discovery of %s: %w", host, context.Cause(ctx))
	}
}

// Invalidate removes the topology of host from the cache, so it is discovered again
func (c *Cache) Invalidate(host string) {
	c.entries.Delete(Key(host))
}

// Key returns the cache key of a host (host[:port]), so equivalent notations share an entry
func Key(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		h, port = strings.Trim(host, "[]"), ""
	}

	h = strings.ToLower(h)
	if port == "" || port == "443" {
		return h
	}

	return net.JoinHostPort(h, port)
}
