package db_test

import (
	"context"
	"os"
	"testing"

	"zeropad-backend/adapters/db"
)

func TestInit_ErrorWhenNoURL(t *testing.T) {
	os.Unsetenv("POSTGRES_URL")
	_, err := db.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when POSTGRES_URL is not set, got nil")
	}
}
