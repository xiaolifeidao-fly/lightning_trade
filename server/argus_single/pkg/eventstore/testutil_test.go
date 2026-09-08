package eventstore

import "encoding/json"

func unmarshalLine(line string, out interface{}) error {
	return json.Unmarshal([]byte(line), out)
}
