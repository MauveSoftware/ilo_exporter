// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MauveSoftware/ilo_exporter/pkg/client"
)

// maxPages limits the number of pages of a collection
const maxPages = 100

// rawCollection is a collection with members which might be expanded
type rawCollection struct {
	Members  []json.RawMessage `json:"Members"`
	NextLink string            `json:"Members@odata.nextLink"`
}

// getMembers retrieves all members of the collection at path. If supported, $expand=. is used to retrieve
// the members with the collection. Members not expanded by the service are retrieved individually.
// The members which could be retrieved are returned, even if an error occurred.
func getMembers[T any](ctx context.Context, s *scrape, path string) ([]T, error) {
	var refs []json.RawMessage

	for page := 0; path != ""; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("collection %s has more than %d pages", path, maxPages)
		}

		col := rawCollection{}
		if err := s.getExpanded(ctx, path, &col); err != nil {
			return nil, err
		}

		refs = append(refs, col.Members...)
		path = col.NextLink
	}

	return resolve[T](ctx, s, refs)
}

// resolve decodes expanded resources and retrieves the resources of plain references
func resolve[T any](ctx context.Context, s *scrape, refs []json.RawMessage) ([]T, error) {
	return collectAll(ctx, refs, func(ctx context.Context, raw json.RawMessage) (T, error) {
		var v T

		var meta struct {
			ID   string `json:"@odata.id"`
			Type string `json:"@odata.type"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			return v, fmt.Errorf("could not parse member: %w", err)
		}

		// every resource has a type, references only contain the ID
		if meta.Type != "" {
			if err := json.Unmarshal(raw, &v); err != nil {
				return v, fmt.Errorf("could not parse %s: %w", meta.ID, err)
			}
			return v, nil
		}

		if meta.ID == "" {
			return v, errors.New("member without @odata.id")
		}

		if err := s.cl.Get(ctx, meta.ID, &v); err != nil {
			return v, err
		}

		return v, nil
	})
}

// getExpanded retrieves a resource with all subordinate resources expanded (if supported).
// If the service rejects $expand, the resource is retrieved again without and $expand is not used anymore.
func (s *scrape) getExpanded(ctx context.Context, path string, obj any) error {
	if !s.expandSupported() {
		return s.cl.Get(ctx, path, obj)
	}

	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}

	err := s.cl.Get(client.WithoutRetry(ctx), path+sep+"$expand=.", obj)

	var he *client.HTTPError
	if !errors.As(err, &he) {
		return err
	}

	switch {
	case he.NotReady:
		// an expanded resource is only ready if all its subordinate resources are ready at the same time, which
		// iLO 5 rarely achieves for storage of drives it manages (their data is kept fresh for a few seconds only).
		// The resource is retrieved without $expand, so the subordinate resources are retrieved (and retried)
		// individually.
		return s.cl.Get(ctx, path, obj)
	case he.StatusCode == http.StatusBadRequest || he.StatusCode == http.StatusNotImplemented:
		s.topology.RejectExpand()
		return s.cl.Get(ctx, path, obj)
	default:
		return err
	}
}

func (s *scrape) expandSupported() bool {
	return s.expand && s.topology != nil && s.topology.ExpandSupported()
}
