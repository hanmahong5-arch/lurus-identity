package cache

import (
	"fmt"
	"testing"
)

func TestCacheKey(t *testing.T) {
	tests := []struct {
		accountID int64
		productID string
		want      string
	}{
		{1, "llm-api", "identity:entitlements:1:llm-api"},
		{42, "quant-trading", "identity:entitlements:42:quant-trading"},
		{9999, "webmail", "identity:entitlements:9999:webmail"},
	}
	for _, tc := range tests {
		got := cacheKey(tc.accountID, tc.productID)
		want := fmt.Sprintf("identity:entitlements:%d:%s", tc.accountID, tc.productID)
		if got != want {
			t.Errorf("cacheKey(%d,%q)=%q, want %q", tc.accountID, tc.productID, got, want)
		}
		if got != tc.want {
			t.Errorf("cacheKey(%d,%q)=%q, want %q", tc.accountID, tc.productID, got, tc.want)
		}
	}
}
