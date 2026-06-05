package run

import (
	"bufio"
	"encoding/json"
	"os"
)

type Event struct {
	Ts    string          `json:"ts"`
	Event string          `json:"event"`
	Actor string          `json:"actor,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func AppendEvent(path string, event Event) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func ReadEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}
	if events == nil {
		events = []Event{}
	}
	return events, scanner.Err()
}
