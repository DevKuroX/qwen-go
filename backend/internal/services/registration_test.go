package services

import (
	"testing"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

func TestNewRegistrationServiceDefaultsPythonPath(t *testing.T) {
	svc := NewRegistrationService("", "script.py")
	if svc.pythonPath != "python3" {
		t.Fatalf("pythonPath = %q, want python3", svc.pythonPath)
	}
}

func TestRegisterSyncFailsWhenNoAccounts(t *testing.T) {
	svc := &RegistrationService{}
	_, err := svc.RegisterSync(models.RegistrationRequest{})
	if err == nil {
		t.Fatal("RegisterSync() error = nil, want error")
	}
}
