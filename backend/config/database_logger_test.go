package config

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestDatabaseLoggerIgnoresExpectedRecordNotFound(t *testing.T) {
	var output bytes.Buffer
	logger := newDatabaseLogger(log.New(&output, "", 0))
	trace := func(err error) {
		logger.Trace(context.Background(), time.Now(), func() (string, int64) {
			return "SELECT * FROM fixtures LIMIT 1", 0
		}, err)
	}

	trace(gorm.ErrRecordNotFound)
	if output.Len() != 0 {
		t.Fatalf("record-not-found query was logged as an error: %q", output.String())
	}

	trace(errors.New("connection failed"))
	if !strings.Contains(output.String(), "connection failed") {
		t.Fatalf("unexpected database errors must remain visible: %q", output.String())
	}
}
