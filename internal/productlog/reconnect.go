package productlog

import "github.com/0disoft/krasis/internal/failurecode"

func init() {
	for _, code := range failurecode.Codes() {
		allowed[code] = true
	}
}
func FaultCode(component, phase, code string) string {
	return failurecode.FaultCode(component, phase, code)
}
func ActivationCode(component, phase, code string) string {
	return failurecode.ActivationCode(component, phase, code)
}
func ReconnectCode(kind, component, phase, code string) string {
	return failurecode.ReconnectCode(kind, component, phase, code)
}
