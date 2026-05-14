package output

import (
	"encoding/json"
	"io"
	"time"
)

type NDJSONEvent struct {
	SchemaVersion string `json:"SchemaVersion"`
	Type          string `json:"Type"`
	Command       string `json:"Command"`
	Timestamp     string `json:"Timestamp"`
	Data          any    `json:"Data"`
	Failure       *Failure `json:"Failure"`
}

type NDJSONWriter struct {
	command string
	encoder *json.Encoder
}

// NewNDJSONWriter creates a writer that emits NDJSON events to w.
func NewNDJSONWriter(w io.Writer, command string) *NDJSONWriter {
	return &NDJSONWriter{command: command, encoder: json.NewEncoder(w)}
}

func (w *NDJSONWriter) writeEvent(eventType string, data any, failure *Failure) error {
	return w.encoder.Encode(NDJSONEvent{
		SchemaVersion: "ags.events.v1",
		Type:          eventType,
		Command:       w.command,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Data:          data,
		Failure:       failure,
	})
}

func (w *NDJSONWriter) WriteStarted(data any) error    { return w.writeEvent("started", data, nil) }
func (w *NDJSONWriter) WriteStdout(chunk string) error  { return w.writeEvent("stdout", map[string]string{"Chunk": chunk}, nil) }
func (w *NDJSONWriter) WriteStderr(chunk string) error  { return w.writeEvent("stderr", map[string]string{"Chunk": chunk}, nil) }
func (w *NDJSONWriter) WriteCompleted(data any) error   { return w.writeEvent("completed", data, nil) }
func (w *NDJSONWriter) WriteFailed(data any, f *Failure) error { return w.writeEvent("failed", data, f) }
