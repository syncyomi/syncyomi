package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"golang.org/x/term"
)

const maxPipedPasswordBytes = 64 << 10

type PasswordResetter interface {
	ResetPassword(ctx context.Context, next string) error
}

type PasswordSource interface {
	NewPassword() (string, error)
}

type pipeSource struct {
	in io.Reader
}

func (s pipeSource) NewPassword() (string, error) {
	line, err := bufio.NewReader(io.LimitReader(s.in, maxPipedPasswordBytes)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}

	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("no password received on stdin")
	}

	return password, nil
}

type terminalSource struct {
	readSecret func(prompt string) (string, error)
}

func (s terminalSource) NewPassword() (string, error) {
	first, err := s.readSecret("New password: ")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	second, err := s.readSecret("Confirm new password: ")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	if first != second {
		return "", errors.New("the two passwords do not match")
	}

	return first, nil
}

func NewPasswordSource(stdin *os.File, out io.Writer) PasswordSource {
	if !term.IsTerminal(int(stdin.Fd())) {
		return pipeSource{in: stdin}
	}

	return terminalSource{readSecret: func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		secret, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(out)
		return string(secret), err
	}}
}

func ResetPassword(ctx context.Context, resetter PasswordResetter, source PasswordSource, out io.Writer) error {
	password, err := source.NewPassword()
	if err != nil {
		return err
	}

	switch err := resetter.ResetPassword(ctx, password); {
	case errors.Is(err, auth.ErrPasswordTooShort):
		return fmt.Errorf("the password must be at least %d characters", auth.MinPasswordLength)
	case errors.Is(err, auth.ErrPasswordTooLong):
		return fmt.Errorf("the password must be at most %d bytes", auth.MaxPasswordBytes)
	case errors.Is(err, auth.ErrNoUser):
		return errors.New("no user exists yet; create the account in the web UI first")
	case err != nil:
		return fmt.Errorf("reset password: %w", err)
	}

	fmt.Fprintln(out, "Password updated. Every web UI session has been signed out.")

	return nil
}

func RunResetPassword(ctx context.Context, resetter PasswordResetter, stdin *os.File, stdout, stderr io.Writer) int {
	if err := ResetPassword(ctx, resetter, NewPasswordSource(stdin, stdout), stdout); err != nil {
		fmt.Fprintln(stderr, "reset-password:", err)
		return 1
	}

	return 0
}
