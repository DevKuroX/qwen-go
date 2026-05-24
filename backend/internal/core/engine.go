package core

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

type RegistrationEngine interface {
	Name() string
	Register(ctx context.Context, req models.RegistrationRequest, onLog func(string)) (*models.Account, error)
}

type MailProvider interface {
	CreateAddress() (string, error)
	PollForActivationLink(ctx context.Context, maxPolls int) (string, error)
}

func newPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 12)
	for i := range b {
		b[i] = chars[rng.Intn(len(chars))]
	}
	return string(b)
}

func newUsername() string {
	const digits = "0123456789"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 6)
	for i := range b {
		b[i] = digits[rng.Intn(len(digits))]
	}
	return fmt.Sprintf("user%s", string(b))
}
