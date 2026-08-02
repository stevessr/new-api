package setting

// DefaultLDCBaseURL is the official native LINUX DO Credit gateway base URL.
// LDCBaseURL may override it for a compatible or self-hosted endpoint.
const DefaultLDCBaseURL = "https://credit.linux.do/epay"

var (
	LDCEnabled      = false
	LDCBaseURL      = DefaultLDCBaseURL
	LDCClientID     = ""
	LDCClientSecret = ""
	LDCPrivateKey   = ""
	LDCMinTopUp     = 1
	LDCNotifyURL    = ""
	LDCReturnURL    = ""
)
