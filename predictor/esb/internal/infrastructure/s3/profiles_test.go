package s3

import (
	"context"
	"testing"
)

func TestNewStoreRejectsInvalidEndpointURL(t *testing.T) {
	for _, endpoint := range []string{"ftp://storage.example", "https://user:secret@storage.example", "http:///missing-host", "https://storage.example?token=unsafe"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewStore(context.Background(), Config{
				Bucket: "company-profiles", Region: "us-east-1", Endpoint: endpoint, MaxProfileBytes: 1024,
			})
			if err == nil {
				t.Fatalf("NewStore accepted invalid endpoint %q", endpoint)
			}
		})
	}
}
