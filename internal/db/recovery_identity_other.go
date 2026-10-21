//go:build !linux

package db

// TargetIdentity is unavailable outside Linux; recovery must fail closed.
func TargetIdentity(string) (string, error) { return "", ErrTargetIdentity }
