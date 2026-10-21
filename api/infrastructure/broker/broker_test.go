package broker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{"key and value", Message{Key: []byte("room-1"), Value: []byte(`{"a":1}`)}},
		{"empty key", Message{Value: []byte("v")}},
		{"empty value", Message{Key: []byte("k")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.msg.Timestamp = time.Unix(0, 1234567890)
			rec := encodeRecord(&tt.msg)

			got, err := decodeRecordBody(rec[sizeFieldLen:])
			if err != nil {
				t.Fatalf("decodeRecordBody: %v", err)
			}
			if string(got.Key) != string(tt.msg.Key) || string(got.Value) != string(tt.msg.Value) {
				t.Fatalf("got key=%q value=%q, want key=%q value=%q", got.Key, got.Value, tt.msg.Key, tt.msg.Value)
			}
			if !got.Timestamp.Equal(tt.msg.Timestamp) {
				t.Fatalf("timestamp = %v, want %v", got.Timestamp, tt.msg.Timestamp)
			}
		})
	}
}

func TestDecodeRecordBodyRejectsCorruptInput(t *testing.T) {
	rec := encodeRecord(&Message{Key: []byte("key"), Value: []byte("value")})
	body := rec[sizeFieldLen:]

	// Claim a key longer than the record.
	body[0], body[1], body[2], body[3] = 0xff, 0xff, 0xff, 0xff
	if _, err := decodeRecordBody(body); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("oversized key: err = %v, want ErrCorruptRecord", err)
	}

	if _, err := decodeRecordBody([]byte{1, 2}); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("short body: err = %v, want ErrCorruptRecord", err)
	}
}

func TestProduceConsumeAndResumeFromCommittedOffset(t *testing.T) {
	dir := t.TempDir()

	b, err := NewBroker(dir)
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	if err := b.CreateTopic("events", 2); err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	if err := b.CreateTopic("events", 2); !errors.Is(err, ErrTopicExists) {
		t.Fatalf("CreateTopic twice: err = %v, want ErrTopicExists", err)
	}

	producer := NewProducer(b, 1)
	for _, v := range []string{"a", "b", "c"} {
		if _, _, err := producer.Produce("events", &Message{Key: []byte("same-key"), Value: []byte(v)}); err != nil {
			t.Fatalf("Produce: %v", err)
		}
	}

	consumer := NewConsumer(b, "group")
	if err := consumer.Subscribe("events"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Same key => same partition => one record per poll, in order.
	var got []string
	for range 2 {
		records, err := consumer.Poll(0)
		if err != nil {
			t.Fatalf("Poll: %v", err)
		}
		for _, r := range records {
			got = append(got, string(r.Value))
		}
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("consumed %v, want [a b]", got)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen: topics are reloaded from disk and the consumer resumes after
	// the committed offset instead of replaying the log.
	b2, err := NewBroker(dir)
	if err != nil {
		t.Fatalf("reopen NewBroker: %v", err)
	}
	defer b2.Close()

	if topics := b2.ListTopics(); len(topics) != 1 || topics[0] != "events" {
		t.Fatalf("ListTopics = %v, want [events]", topics)
	}

	c2 := NewConsumer(b2, "group")
	if err := c2.Subscribe("events"); err != nil {
		t.Fatalf("Subscribe after reopen: %v", err)
	}

	records, err := c2.Poll(0)
	if err != nil {
		t.Fatalf("Poll after reopen: %v", err)
	}
	if len(records) != 1 || string(records[0].Value) != "c" {
		t.Fatalf("after reopen consumed %d records (%v), want only [c]", len(records), records)
	}

	if records, _ := c2.Poll(0); len(records) != 0 {
		t.Fatalf("expected no more records, got %d", len(records))
	}
}

func TestConsumerSkipsCorruptPartition(t *testing.T) {
	dir := t.TempDir()

	b, err := NewBroker(dir)
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	if err := b.CreateTopic("t", 1); err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A record whose size field points far beyond the end of the file.
	garbage := []byte{0x7f, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3}
	if err := os.WriteFile(filepath.Join(dir, "t", "partition-0.log"), garbage, 0o644); err != nil {
		t.Fatal(err)
	}

	b, err = NewBroker(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer b.Close()

	c := NewConsumer(b, "g")
	if err := c.Subscribe("t"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, err := c.Poll(0); err != nil {
		t.Fatalf("Poll should skip the corrupt record, got %v", err)
	}
	if records, err := c.Poll(0); err != nil || len(records) != 0 {
		t.Fatalf("second Poll = %d records, %v; want 0, nil", len(records), err)
	}
}
