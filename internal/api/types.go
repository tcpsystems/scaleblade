package api

import "encoding/json"

// The structs below model the response shapes observed during a read-only
// probe of the portal (2026-10-19). Unknown or provider-specific nested values
// are kept as json.RawMessage. The CLI's --json mode prints the original raw
// data instead of these structs, so no fields are lost.

// Me is GET /v1/auth/me.
type Me struct {
	SessionID           string `json:"session_id"`
	ActorID             string `json:"actor_id"`
	EffectiveActorID    string `json:"effective_actor_id"`
	EffectiveActorEmail string `json:"effective_actor_email"`
	Role                string `json:"role"`
	Impersonating       bool   `json:"impersonating"`
	ExpiresAt           string `json:"expires_at"`
}

// MemberView is a hydrated project member in GET /v1/portal/projects/{id}.
type MemberView struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

// Project is an item from GET /v1/portal/projects.
type Project struct {
	ID                string          `json:"_id"`
	Label             string          `json:"label"`
	OwnerAccountID    string          `json:"owner_account_id"`
	Members           json.RawMessage `json:"members"`
	OwnershipTransfer json.RawMessage `json:"ownership_transfer"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	ArchivedAt        *string         `json:"archived_at"`
	MemberViews       []MemberView    `json:"member_views,omitempty"`
}

// Server is a bare-metal compute resource from /v1/portal/compute.
//
// The account used for the observed probe had no bare-metal servers, so this
// shape is inferred from portal bundles and neighbouring resources. Use --json
// for lossless output.
type Server struct {
	ID               string  `json:"_id"`
	Label            string  `json:"label"`
	Hostname         string  `json:"hostname"`
	OperationalState string  `json:"operational_state"`
	BillingState     string  `json:"billing_state"`
	ProjectID        string  `json:"project_id"`
	PrimaryIPAddress string  `json:"primary_ip_address"`
	LocationLabel    string  `json:"location_label"`
	CreatedAt        string  `json:"created_at"`
	ArchivedAt       *string `json:"archived_at"`
}

// OSProfile is an item from GET /v1/portal/compute/os-profiles.
type OSProfile struct {
	ID           string          `json:"_id"`
	Family       string          `json:"family"`
	Version      string          `json:"version"`
	Kind         string          `json:"kind"`
	Architecture string          `json:"architecture"`
	InputSchema  json.RawMessage `json:"input_schema"`
	Installation json.RawMessage `json:"installation"`
}

// VM is a virtualization virtual machine (observed shape).
type VM struct {
	ID                    string `json:"_id"`
	Label                 string `json:"label"`
	Hostname              string `json:"hostname"`
	OperationalState      string `json:"operational_state"`
	BillingState          string `json:"billing_state"`
	BillingTerm           string `json:"billing_term"`
	VCPU                  int64  `json:"vcpu"`
	MemoryMB              int64  `json:"memory_mb"`
	StorageGB             int64  `json:"storage_gb"`
	TransferTB            int64  `json:"transfer_tb"`
	TransferUnlimited     bool   `json:"transfer_unlimited"`
	HourlyPriceMicropence int64  `json:"hourly_price_micropence"`
	RecurringPricePence   int64  `json:"recurring_price_pence"`
	CreatedAt             string `json:"created_at"`
	ReinstallTaskID       string `json:"reinstall_task_id"`
	PrimaryIPAddress      string `json:"primary_ip_address"`
	OSLabel               string `json:"os_label"`
	LocationLabel         string `json:"location_label"`
	RenewsAt              string `json:"renews_at"`
}

// Metric is one sample from GET
// /v1/portal/virtualization/virtual-machines/{id}/metrics?range=...
type Metric struct {
	Time           float64 `json:"time"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryBytes    int64   `json:"memory_bytes"`
	MemoryMaxBytes int64   `json:"memory_max_bytes"`
	InboundBPS     float64 `json:"inbound_bps"`
	OutboundBPS    float64 `json:"outbound_bps"`
}

// Deployment is GET /v1/portal/virtualization/virtual-machines/{id}/deployment.
type Deployment struct {
	VirtualMachine VM   `json:"virtual_machine"`
	Task           Task `json:"task"`
}

// Task is an asynchronous provider operation (observed shape).
type Task struct {
	ID               string     `json:"_id"`
	DisplayID        string     `json:"display_id"`
	Type             string     `json:"type"`
	State            string     `json:"state"`
	ActorID          string     `json:"actor_id"`
	EffectiveActorID string     `json:"effective_actor_id"`
	ResourceType     string     `json:"resource_type"`
	ResourceID       string     `json:"resource_id"`
	AgentID          *string    `json:"agent_id"`
	IdempotencyKey   string     `json:"idempotency_key"`
	CorrelationID    string     `json:"correlation_id"`
	OperationGroup   string     `json:"operation_group"`
	Visibility       string     `json:"visibility"`
	ParentTaskID     *string    `json:"parent_task_id"`
	ParentStepID     *string    `json:"parent_step_id"`
	RetryOfTaskID    *string    `json:"retry_of_task_id"`
	Progress         float64    `json:"progress"`
	Steps            []TaskStep `json:"steps"`
	CreatedAt        string     `json:"created_at"`
	UpdatedAt        string     `json:"updated_at"`
	StartedAt        string     `json:"started_at"`
	FinishedAt       string     `json:"finished_at"`
	WaitingReason    *string    `json:"waiting_reason"`
	ResumeAfter      *string    `json:"resume_after"`
	ArchivedAt       *string    `json:"archived_at"`
	Cancellable      bool       `json:"cancellable"`
}

// TaskStep is one step inside a Task.
type TaskStep struct {
	ID               string          `json:"id"`
	Key              string          `json:"key"`
	State            string          `json:"state"`
	Executor         string          `json:"executor"`
	Attempt          int64           `json:"attempt"`
	AgentID          *string         `json:"agent_id"`
	ChildTaskIDs     []string        `json:"child_task_ids"`
	Result           json.RawMessage `json:"result"`
	ResumeAfter      *string         `json:"resume_after"`
	SanitizedMessage *string         `json:"sanitized_message"`
	StartedAt        string          `json:"started_at"`
	FinishedAt       string          `json:"finished_at"`
	TimeoutSeconds   int64           `json:"timeout_seconds"`
	WaitingReason    *string         `json:"waiting_reason"`
}

// Allocation is a network allocation (observed shape).
type Allocation struct {
	ID                  string `json:"_id"`
	Address             string `json:"address"`
	Kind                string `json:"kind"`
	State               string `json:"state"`
	ProjectID           string `json:"project_id"`
	SubnetID            string `json:"subnet_id"`
	ServiceID           string `json:"service_id"`
	ServiceType         string `json:"service_type"`
	OwnershipSource     string `json:"ownership_source"`
	OwnershipStartedAt  string `json:"ownership_started_at"`
	OwnershipEndedAt    string `json:"ownership_ended_at"`
	RetainedServiceID   string `json:"retained_service_id"`
	RetainedServiceType string `json:"retained_service_type"`
	ReservationReason   string `json:"reservation_reason"`
	TaskID              string `json:"task_id"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	ArchivedAt          string `json:"archived_at"`
}

// AllocationPage is GET /v1/portal/network.
type AllocationPage struct {
	Items  []Allocation `json:"items"`
	Total  int64        `json:"total"`
	Offset int64        `json:"offset"`
	Limit  int64        `json:"limit"`
}

// IPv4Price is one floating IPv4 pricing tier.
type IPv4Price struct {
	Enabled    *bool           `json:"enabled"`
	PricePence *int64          `json:"price_pence"`
	Protection json.RawMessage `json:"protection"`
}

// Pricing is GET /v1/portal/network/pricing.
type Pricing struct {
	Configured   bool            `json:"configured"`
	FloatingIPv4 []IPv4Price     `json:"floating_ipv4"`
	IPv6Prefixes json.RawMessage `json:"ipv6_prefixes"`
	UpdatedAt    string          `json:"updated_at"`
}

// InvoiceLine is one line on an invoice.
type InvoiceLine struct {
	Description  string `json:"description"`
	GrossPence   int64  `json:"gross_pence"`
	NetPence     int64  `json:"net_pence"`
	Quantity     int64  `json:"quantity"`
	ServiceID    string `json:"service_id"`
	ServiceType  string `json:"service_type"`
	UnitNetPence int64  `json:"unit_net_pence"`
	VATPence     int64  `json:"vat_pence"`
}

// Invoice is a billing invoice (observed shape).
type Invoice struct {
	ID                            string          `json:"_id"`
	DisplayID                     string          `json:"display_id"`
	AccountID                     string          `json:"account_id"`
	ProjectID                     string          `json:"project_id"`
	State                         string          `json:"state"`
	InvoiceType                   string          `json:"invoice_type"`
	Currency                      string          `json:"currency"`
	Lines                         []InvoiceLine   `json:"lines"`
	NetPence                      int64           `json:"net_pence"`
	VATPence                      int64           `json:"vat_pence"`
	GrossPence                    int64           `json:"gross_pence"`
	CreditAppliedPence            int64           `json:"credit_applied_pence"`
	ProviderPaidPence             int64           `json:"provider_paid_pence"`
	RefundedGrossPence            int64           `json:"refunded_gross_pence"`
	BillingSnapshot               json.RawMessage `json:"billing_snapshot"`
	DueAt                         *string         `json:"due_at"`
	ExpiresAt                     *string         `json:"expires_at"`
	IssuedAt                      string          `json:"issued_at"`
	PaidAt                        *string         `json:"paid_at"`
	IdempotencyKey                string          `json:"idempotency_key"`
	AggregationKey                *string         `json:"aggregation_key"`
	ExternalReference             json.RawMessage `json:"external_reference"`
	PrepaidConsumedBySubscription *string         `json:"prepaid_consumed_by_subscription_id"`
	RecoveryHold                  json.RawMessage `json:"recovery_hold"`
	AutoCollectionPaused          bool            `json:"auto_collection_paused"`
	CreatedAt                     string          `json:"created_at"`
	UpdatedAt                     string          `json:"updated_at"`
	ArchivedAt                    *string         `json:"archived_at"`
}

// Subscription is a recurring billing subscription.
type Subscription struct {
	ID                      string          `json:"_id"`
	AccountID               string          `json:"account_id"`
	ProjectID               string          `json:"project_id"`
	ServiceID               string          `json:"service_id"`
	ServiceType             string          `json:"service_type"`
	State                   string          `json:"state"`
	Period                  string          `json:"period"`
	CurrentNetPricePence    int64           `json:"current_net_price_pence"`
	ListedNetPricePence     int64           `json:"listed_net_price_pence"`
	PricingAdjustment       json.RawMessage `json:"pricing_adjustment"`
	PrepaidInvoiceID        *string         `json:"prepaid_invoice_id"`
	CancellationEffectiveAt *string         `json:"cancellation_effective_at"`
	RestorationTaskID       *string         `json:"restoration_task_id"`
	RenewsAt                *string         `json:"renews_at"`
	CreatedAt               string          `json:"created_at"`
	UpdatedAt               string          `json:"updated_at"`
	ArchivedAt              *string         `json:"archived_at"`
}

// BillingSummary is GET /v1/portal/billing.
type BillingSummary struct {
	Invoices           []Invoice      `json:"invoices"`
	Subscriptions      []Subscription `json:"subscriptions"`
	CreditBalancePence int64          `json:"credit_balance_pence"`
}

// Ticket is a support ticket.
//
// No tickets existed for the observed account, so only common fields are
// modelled. --json preserves the server response exactly.
type Ticket struct {
	ID          string  `json:"_id"`
	DisplayID   string  `json:"display_id"`
	Subject     string  `json:"subject"`
	Title       string  `json:"title"`
	State       string  `json:"state"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
	Department  string  `json:"department"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	LastReplyAt *string `json:"last_reply_at"`
}

// Department is GET /v1/portal/support/departments.
type Department struct {
	ID                 string `json:"_id"`
	Key                string `json:"key"`
	Label              string `json:"label"`
	CustomerSelectable bool   `json:"customer_selectable"`
	Protected          bool   `json:"protected"`
}

// TicketStatus is GET /v1/portal/support/statuses.
type TicketStatus struct {
	ID             string `json:"_id"`
	Key            string `json:"key"`
	Label          string `json:"label"`
	Color          string `json:"color"`
	Semantic       string `json:"semantic"`
	Protected      bool   `json:"protected"`
	ClosedBehavior bool   `json:"closed_behavior"`
}

// IncidentEligibility is GET
// /v1/portal/support/tickets/incident-eligibility.
type IncidentEligibility struct {
	Eligible        bool   `json:"eligible"`
	Reason          string `json:"reason"`
	MinimumMRRPence int64  `json:"minimum_mrr_pence"`
}

// Notice is one active portal notice.
type Notice struct {
	ID        string `json:"_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Level     string `json:"level"`
	CreatedAt string `json:"created_at"`
}

// NotificationPreferences is GET /v1/portal/notifications/preferences.
type NotificationPreferences struct {
	AccountID          string `json:"account_id"`
	InformationalEmail bool   `json:"informational_email"`
	MarketingEmail     bool   `json:"marketing_email"`
}

// VirtualizationCategory is GET /v1/portal/virtualization/categories.
type VirtualizationCategory struct {
	ID          string `json:"_id"`
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	Description string `json:"description"`
	SortOrder   int64  `json:"sort_order"`
	Enabled     bool   `json:"enabled"`
}

// Image is GET /v1/portal/virtualization/images.
type Image struct {
	ID                      string  `json:"_id"`
	Family                  string  `json:"family"`
	Version                 string  `json:"version"`
	Label                   string  `json:"label"`
	Architecture            string  `json:"architecture"`
	Enabled                 bool    `json:"enabled"`
	CustomerLicenceRequired bool    `json:"customer_licence_required"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
	ArchivedAt              *string `json:"archived_at"`
}

// Location is GET /v1/portal/virtualization/locations.
type Location struct {
	ID          string `json:"_id"`
	Label       string `json:"label"`
	Code        string `json:"code"`
	CountryCode string `json:"country_code"`
}

// Datacentre is GET /v1/portal/virtualization/datacentres.
type Datacentre struct {
	ID         string `json:"_id"`
	Label      string `json:"label"`
	Code       string `json:"code"`
	LocationID string `json:"location_id"`
}
