//go:build windows

package wininput

func CurrentDisplayGeometry() DisplayGeometry {
	return DisplayGeometry{Count: systemMetric(smCMONITORS), Left: systemMetric(smXVIRTUALSCREEN), Top: systemMetric(smYVIRTUALSCREEN), Width: systemMetric(smCXVIRTUALSCREEN), Height: systemMetric(smCYVIRTUALSCREEN)}
}
