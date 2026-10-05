package residentagent

import "time"

func (connection *disconnectConnection) SetReadDetailObserver(observer func(int, time.Time)) bool {
	source, ok := connection.Connection.(interface {
		SetReadDetailObserver(func(int, time.Time)) bool
	})
	return ok && source.SetReadDetailObserver(observer)
}
