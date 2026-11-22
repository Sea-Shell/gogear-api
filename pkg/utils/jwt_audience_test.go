package utils

import (
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

func TestValidateAuthAudiences(t *testing.T) {
	cases := []struct {
		name          string
		client, admin string
		wantErr       bool
	}{
		{"distinct", "client", "admin", false},
		{"equal", "same", "same", true},
		{"equal after trimming", "same", " same ", true},
		{"empty client", "", "admin", true},
		{"empty admin", "client", "", true},
		{"blank admin", "client", "   ", true},
		{"both empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAuthAudiences(&models.Auth{JWTAudience: tc.client, JWTAdminAudience: tc.admin})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
