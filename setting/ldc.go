package setting

const (
	DefaultLDCBaseURL = "https://credit.linux.do/epay"
)

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
