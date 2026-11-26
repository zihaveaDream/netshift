package netcfg

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// awaitAdapterWith polls listFn until pred accepts the adapter named name, and
// calls onUpdate every time that adapter's state changes.
//
// Switching an adapter to DHCP or enabling it takes effect immediately, but the
// old address is released and the new one assigned a few seconds later, so the
// first sample is taken right away and then every interval. A missing adapter or
// a failing listFn does not abort the wait: the next sample retries. It returns
// the last successful sample and whether pred was met, so callers can report a
// timeout with the freshest state instead of nothing.
func awaitAdapterWith(ctx context.Context, name string, interval time.Duration, listFn func() ([]Adapter, error), pred func(Adapter) bool, onUpdate func([]Adapter, Adapter)) ([]Adapter, Adapter, bool) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	var (
		lastList   []Adapter
		lastTarget Adapter
		lastPrint  string
		haveTarget bool
	)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		list, err := listFn()
		if err == nil {
			lastList = list
			for _, ad := range list {
				if ad.Name != name {
					continue
				}
				lastTarget = ad
				haveTarget = true
				break
			}
		}
		if haveTarget {
			if pred != nil && pred(lastTarget) {
				return lastList, lastTarget, true
			}
			if fp := adapterFingerprint(lastTarget); fp != lastPrint {
				lastPrint = fp
				if onUpdate != nil {
					onUpdate(lastList, lastTarget)
				}
			}
		}
		select {
		case <-ctx.Done():
			return lastList, lastTarget, false
		case <-ticker.C:
		}
	}
}

// adapterFingerprint is the state a caller showing an adapter cares about; a
// change means the UI has something new to draw.
func adapterFingerprint(a Adapter) string {
	return fmt.Sprintf("%v|%v|%s|%s|%s|%s|%s", a.Up, a.DHCP, a.MAC,
		strings.Join(a.IPv4, ","), strings.Join(a.Masks, ","),
		strings.Join(a.Gateways, ","), strings.Join(a.DNS, ","))
}
