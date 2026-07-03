package apikey

// TierLimits gates API usage per roadmap 3.6 (request quota) and the
// bandwidth-tracking extension. A minimal stand-in for the full
// tierLimits(tier) concept in roadmap 1.5, scoped to what /api/pads/*
// needs today.
type TierLimits struct {
	DailyRequestQuota  int64
	DailyBandwidthByte int64
}

var tierLimits = map[string]TierLimits{
	"paid": {DailyRequestQuota: 100_000, DailyBandwidthByte: 50 * 1024 * 1024 * 1024}, // 50 GB/day
	"free": {DailyRequestQuota: 0, DailyBandwidthByte: 0},                             // API access is paid-only; kept for safety if a key's owner is downgraded
}

func Limits(tier string) TierLimits {
	if l, ok := tierLimits[tier]; ok {
		return l
	}
	return tierLimits["free"]
}
