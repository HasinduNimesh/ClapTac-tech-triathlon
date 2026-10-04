package authorization

const (
	RoleStoreManager = "STORE_MANAGER"
	RoleDispatcher   = "DISPATCHER"
	RoleLoader       = "LOADER"
	RoleDriver       = "DRIVER"

	PermOrderCreate            = "order:create"
	PermOrderViewOwn           = "order:view-own"
	PermOrderViewAll           = "order:view-all"
	PermOrderDefer             = "order:defer"
	PermReceiptConfirm         = "receipt:confirm"
	PermDeliveryIssueCreate    = "delivery:issue-create"
	PermPlanCreate             = "plan:create"
	PermPlanView               = "plan:view"
	PermPlanUpdate             = "plan:update"
	PermAllocationCreate       = "allocation:create"
	PermAllocationUpdate       = "allocation:update"
	PermFleetView              = "fleet:view"
	PermFleetUpdate            = "fleet:update"
	PermOrdersReadInternal     = "orders:read-internal"
	PermFleetReadInternal      = "fleet:read-internal"
	PermOutletsReadInternal    = "outlets:read-internal"
	PermDeliveryViewAll        = "delivery:view-all"
	PermLoadingView            = "loading:view"
	PermLoadingViewAll         = "loading:view-all"
	PermLoadingUpdate          = "loading:update"
	PermLoadingIssue           = "loading:issue"
	PermLoadingReady           = "loading:ready"
	PermLoadingReadInternal    = "loading:read-internal"
	PermLoadingDecide          = "loading:decide"
	PermPlansReadInternal      = "plans:read-internal"
	PermDeliveriesReadInternal = "deliveries:read-internal"
	PermTripViewAssigned       = "trip:view-assigned"
	PermDeliveryViewAssigned   = "delivery:view-assigned"
	PermDeliveryView           = "delivery:view"
	PermDeliveryStart          = "delivery:start"
	PermDeliveryUpdate         = "delivery:update"
	PermDeliveryProof          = "delivery:proof"
	PermDeliveryComplete       = "delivery:complete"
	PermDeliverySync           = "delivery:sync"
	PermProofCreate            = "proof:create"
	PermAuditWrite             = "audit:write"
	PermAuditRead              = "audit:read"
	PermMasterDataUpdate       = "masterdata:update"
	PermPolicyReadInternal     = "policy:read-internal"
	PermPlanAcknowledge        = "plan:acknowledge"
	PermDashboardManageOwn     = "dashboard:manage-own"
)

var RolePermissions = map[string][]string{
	RoleStoreManager: {
		PermOrderCreate,
		PermOrderViewOwn,
		PermReceiptConfirm,
		PermDeliveryIssueCreate,
		PermDashboardManageOwn,
	},
	RoleDispatcher: {
		PermOrderViewAll,
		PermPlanCreate,
		PermPlanView,
		PermPlanUpdate,
		PermAllocationCreate,
		PermAllocationUpdate,
		PermOrderDefer,
		PermFleetView,
		PermFleetUpdate,
		PermDeliveryViewAll,
		PermLoadingViewAll,
		PermLoadingDecide,
		PermAuditRead,
		PermMasterDataUpdate,
		PermDashboardManageOwn,
	},
	RoleLoader: {
		PermPlanAcknowledge,
		PermLoadingView,
		PermLoadingUpdate,
		PermLoadingIssue,
		PermLoadingReady,
	},
	RoleDriver: {
		PermPlanAcknowledge,
		PermTripViewAssigned,
		PermDeliveryViewAssigned,
		PermDeliveryView,
		PermDeliveryStart,
		PermDeliveryUpdate,
		PermDeliveryProof,
		PermDeliveryComplete,
		PermDeliverySync,
		PermProofCreate,
	},
}

func HasPermission(roles []string, permission string) bool {
	for _, role := range roles {
		for _, p := range RolePermissions[role] {
			if p == permission {
				return true
			}
		}
	}
	return false
}
