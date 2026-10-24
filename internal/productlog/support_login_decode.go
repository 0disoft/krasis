package productlog

import (
	"encoding/json"

	"github.com/0disoft/krasis/internal/logindiag"
)

type boundedLoginRuntime logindiag.Runtime
type boundedSupportSourceHistory logindiag.SourceHistory
type boundedSupportSourceExits []logindiag.SourceExit

func (v boundedLoginRuntime) Valid() bool { return logindiag.Runtime(v).Valid() }

func (target *boundedLoginRuntime) UnmarshalJSON(data []byte) error {
	var wire struct {
		logindiag.Runtime
		Source *boundedSupportSourceHistory `json:"source"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Source != nil {
		value := logindiag.SourceHistory(*wire.Source)
		wire.Runtime.Source = &value
	}
	*target = boundedLoginRuntime(wire.Runtime)
	return nil
}

func (target *boundedSupportSourceHistory) UnmarshalJSON(data []byte) error {
	var wire struct {
		logindiag.SourceHistory
		Exits boundedSupportSourceExits `json:"exits"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	wire.SourceHistory.Exits = wire.Exits
	*target = boundedSupportSourceHistory(wire.SourceHistory)
	return nil
}

func (target *boundedSupportSourceExits) UnmarshalJSON(data []byte) error {
	values, err := decodeSupportArray(data, logindiag.MaxSourceExits, func(d *json.Decoder) (logindiag.SourceExit, error) {
		var value logindiag.SourceExit
		err := d.Decode(&value)
		return value, err
	})
	if err == nil {
		*target = values
	}
	return err
}
