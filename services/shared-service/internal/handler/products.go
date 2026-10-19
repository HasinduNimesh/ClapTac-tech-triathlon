package handler

import (
	"net/http"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

const maxProductIDs = 200

// products lists active catalog products. `ids` is a comma-separated lookup (used by order-service to price an
// order from its lines); otherwise the list is the caller's own outlet range, or any outlet's for staff who plan
// across outlets, optionally narrowed to one brand.
func (h Handler) products(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var ids []string
	if raw := strings.TrimSpace(q.Get("ids")); raw != "" {
		seen := map[string]bool{}
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if !store.ValidProductID(id) {
				apierrors.BadRequest(w, "invalid product id")
				return
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > maxProductIDs {
			apierrors.BadRequest(w, "too many product ids")
			return
		}
	}
	outletID := strings.TrimSpace(q.Get("outlet_id"))
	if profile, _ := authorization.ProfileFrom(r.Context()); profile != nil && !authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) && len(profile.OutletIDs) > 0 {
		// A store manager only ever sees their own outlet's range.
		outletID = profile.OutletIDs[0]
	}
	items, err := h.Store.Products(r.Context(), ids, outletID, strings.TrimSpace(q.Get("brand")))
	if err != nil {
		apierrors.Internal(w, "product lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
