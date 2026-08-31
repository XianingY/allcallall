package compliance

import (
	"regexp"
	"strings"
)

// icpRE matches a Chinese ICP filing number, e.g. "京ICP备12345678号-1".
// Province abbreviation + "ICP备" + 6-12 digits + "号" + optional "-N".
var icpRE = regexp.MustCompile(`^[\x{4e00}-\x{9fa5}]+ICP备\d{6,12}号(-\d+)?$`)

// ValidateICPFormat checks the syntactic validity of an ICP filing number.
func ValidateICPFormat(number string) bool {
	return icpRE.MatchString(strings.TrimSpace(number))
}

// ICPRegistry holds the set of ICP numbers this service is authorized under.
type ICPRegistry struct {
	registered map[string]bool
}

// NewICPRegistry builds a registry from the given filing numbers.
func NewICPRegistry(numbers ...string) *ICPRegistry {
	m := make(map[string]bool, len(numbers))
	for _, n := range numbers {
		n = strings.TrimSpace(n)
		if n != "" {
			m[n] = true
		}
	}
	return &ICPRegistry{registered: m}
}

// IsRegistered reports whether the given ICP number is in the authorized set.
func (r *ICPRegistry) IsRegistered(number string) bool {
	return r.registered[strings.TrimSpace(number)]
}

// FromEnvCSV builds a registry from a comma-separated ICP number list.
func FromEnvCSV(csv string) *ICPRegistry {
	parts := strings.Split(csv, ",")
	nums := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nums = append(nums, strings.TrimSpace(p))
		}
	}
	return NewICPRegistry(nums...)
}
