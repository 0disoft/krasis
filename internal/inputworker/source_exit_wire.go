package inputworker

import "github.com/0disoft/krasis/internal/logindiag"

var sourceExitVocabulary = [][]string{
	{"not_started", "ok", "inspection", "authority", "protocol", "cleanup", "cancelled", "failed"},
	{"not_started", "stopped", "cancelled", "eof", "truncated", "timeout", "closed", "invalid", "authority", "failed"},
	{"not_started", "joined", "failed"},
	{"not_started", "joined", "unjoined"},
	{"not_started", "joined", "unjoined"},
	{"not_sent", "clean_sent", "failed_sent", "delivery_failed"},
}

func sourceReportState(status byte) string {
	return [...]string{"ok", "failed", "recovery"}[status]
}

func encodeSourceExit(v logindiag.SourceExit, status byte) ([8]byte, error) {
	var encoded [8]byte
	if status > 2 {
		return encoded, ErrProtocol
	}
	v.Report = sourceReportState(status)
	if !v.ValidDetails() || v.Time != "" || status != 2 && (v.Sender == "unjoined" || v.Interrupt == "unjoined" || v.Capture == "failed") {
		return encoded, ErrProtocol
	}
	for i, field := range []string{v.Command, v.Reader, v.Capture, v.Sender, v.Interrupt, v.Retirement} {
		found := false
		for code, value := range sourceExitVocabulary[i] {
			if value == field {
				encoded[i] = byte(code)
				found = true
				break
			}
		}
		if !found {
			return [8]byte{}, ErrProtocol
		}
	}
	return encoded, nil
}

func decodeSourceExit(encoded []byte, status byte) (*logindiag.SourceExit, error) {
	if len(encoded) != 8 || status > 2 || encoded[6] != 0 || encoded[7] != 0 {
		return nil, ErrProtocol
	}
	var fields [6]string
	for i := range fields {
		if int(encoded[i]) >= len(sourceExitVocabulary[i]) {
			return nil, ErrProtocol
		}
		fields[i] = sourceExitVocabulary[i][encoded[i]]
	}
	v := &logindiag.SourceExit{Command: fields[0], Reader: fields[1], Capture: fields[2], Sender: fields[3], Interrupt: fields[4], Retirement: fields[5], Report: sourceReportState(status)}
	if _, err := encodeSourceExit(*v, status); err != nil {
		return nil, err
	}
	return v, nil
}
