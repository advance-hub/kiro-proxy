package kiro

import (
	"crypto/sha256"
	"fmt"

	"kiro-go/internal/model"
)

// GenerateMachineID 生成 machineId
func GenerateMachineID(cred *model.KiroCredentials, cfg *model.Config) string {
	if cred.MachineID != "" {
		return cred.MachineID
	}
	input := cred.RefreshToken
	if input == "" {
		input = "default"
	}
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)
}
