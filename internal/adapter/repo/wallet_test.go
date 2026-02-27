package repo

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateOrderNo(t *testing.T) {
	// Format: "LO" + YYYYMMDD + 6-digit suffix
	orderNo := GenerateOrderNo(1234)
	if !strings.HasPrefix(orderNo, "LO") {
		t.Errorf("GenerateOrderNo: expected LO prefix, got %q", orderNo)
	}
	// total length: 2 + 8 + 6 = 16
	if len(orderNo) != 16 {
		t.Errorf("GenerateOrderNo: len=%d, want 16; got %q", len(orderNo), orderNo)
	}
	// Date portion must be parseable
	datePart := orderNo[2:10]
	if _, err := time.Parse("20060102", datePart); err != nil {
		t.Errorf("GenerateOrderNo: date part %q not parseable: %v", datePart, err)
	}
}

func TestGenerateOrderNo_Suffix(t *testing.T) {
	// Suffix = accountID % 1000000, zero-padded to 6 digits
	tests := []struct {
		accountID int64
		wantSuffix string
	}{
		{1, "000001"},
		{999999, "999999"},
		{1000000, "000000"},
		{1000001, "000001"},
	}
	for _, tc := range tests {
		no := GenerateOrderNo(tc.accountID)
		suffix := no[10:]
		if suffix != tc.wantSuffix {
			t.Errorf("GenerateOrderNo(%d) suffix=%q, want %q", tc.accountID, suffix, tc.wantSuffix)
		}
	}
}
