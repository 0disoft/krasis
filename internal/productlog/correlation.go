package productlog

import "encoding/hex"

func validHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// Connection updates diagnostic context only; it never grants peer authority.
func (logger *Logger) Connection(id string) {
	if logger == nil {
		return
	}
	if !validHex(id, 64) {
		id = ""
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.connection = id
}
