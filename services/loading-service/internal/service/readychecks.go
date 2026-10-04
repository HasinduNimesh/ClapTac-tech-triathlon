package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

// ReadyChecks are the loader's departure checks: the chilled-zone reading and the
// door seal number.
type ReadyChecks struct {
	ChilledTemperatureC *float64
	SealNumber          string
}

// validateReadyChecks decides whether the checks that came with a ready request
// are enough to release a trip. A refrigerated vehicle needs both a chilled-zone
// reading inside the range and a seal number; for any other vehicle they are
// optional but still validated when given. The result carries the trimmed seal.
func validateReadyChecks(capability string, c ReadyChecks) (ReadyChecks, error) {
	c.SealNumber = strings.TrimSpace(c.SealNumber)
	if len(c.SealNumber) > 40 {
		return c, fmt.Errorf("invalid: sealNumber too long")
	}
	if t := c.ChilledTemperatureC; t != nil && (math.IsNaN(*t) || *t < -30 || *t > 30) {
		return c, fmt.Errorf("invalid: chilledTemperatureC")
	}
	if !refrigerated(capability) {
		return c, nil
	}
	if c.ChilledTemperatureC == nil {
		return c, fmt.Errorf("invalid: chilledTemperatureC is required before a refrigerated vehicle departs")
	}
	if c.SealNumber == "" {
		return c, fmt.Errorf("invalid: sealNumber is required before a refrigerated vehicle departs")
	}
	if t := *c.ChilledTemperatureC; t < domain.ChilledZoneMinC || t > domain.ChilledZoneMaxC {
		return c, fmt.Errorf("conflict: chilled zone reads %.1f °C; it must be %.0f–%.0f °C before departure", t, domain.ChilledZoneMinC, domain.ChilledZoneMaxC)
	}
	return c, nil
}
