package types

import "fmt"

type EligibilityStatus string

const (
	EligibilityStatusActive   EligibilityStatus = "active"
	EligibilityStatusInactive EligibilityStatus = "inactive"
)

func (es EligibilityStatus) ToString() string {
	return string(es)
}

func EligibilityStatusFromString(s string) (EligibilityStatus, error) {
	switch s {
	case EligibilityStatusActive.ToString():
		return EligibilityStatusActive, nil
	case EligibilityStatusInactive.ToString():
		return EligibilityStatusInactive, nil
	default:
		return "", fmt.Errorf("invalid eligibility status: %s", s)
	}
}
